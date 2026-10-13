package io.github.slaynaw.wakeonlan.data

import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.annotation.RequiresApi
import io.github.slaynaw.wakeonlan.core.agent2.DeviceIdentity
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import java.math.BigInteger
import java.security.GeneralSecurityException
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.security.spec.ECGenParameterSpec
import javax.security.auth.x500.X500Principal

/**
 * Identité wolagent/2 de ce téléphone (docs/PROTOCOLE-V2.md, §2.3) : clé ECDSA P-256 créée dans le Keystore Android
 * (StrongBox si le téléphone en a un, sinon l'environnement sécurisé du processeur), jamais exportable, utilisable sans
 * authentification de l'utilisateur (la surveillance des PC en a besoin). Le certificat créé par le Keystore la
 * présente dans TLS : l'agent n'en regarde que la clé.
 *
 * Android 10 au moins (TLS 1.3) : avant, il n'y en a pas, et le téléphone reste en wolagent/1 avec les PC qui
 * l'acceptent (§13). Elle ne figure dans aucune sauvegarde : un nouveau téléphone a une nouvelle identité.
 */
class KeystoreIdentity(private val alias: String = ALIAS) {
    @Volatile
    private var cached: DeviceIdentity? = null

    /** Identité de ce téléphone, créée au premier appel ; null avant Android 10 ou si le Keystore échoue. */
    @Synchronized
    fun get(): DeviceIdentity? {
        cached?.let { return it }
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) return null
        return try {
            (load() ?: create()).also { cached = it }
        } catch (e: GeneralSecurityException) {
            DiagnosticLog.w("identité", "identité wolagent/2 indisponible", e)
            null
        } catch (e: java.security.ProviderException) {
            DiagnosticLog.w("identité", "identité wolagent/2 indisponible", e)
            null
        } catch (e: java.io.IOException) {
            DiagnosticLog.w("identité", "identité wolagent/2 indisponible", e)
            null
        }
    }

    @RequiresApi(Build.VERSION_CODES.Q)
    private fun load(): DeviceIdentity? {
        val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        val key = keyStore.getKey(alias, null) as? PrivateKey ?: return null
        val certificate = keyStore.getCertificate(alias) as? X509Certificate ?: return null
        return DeviceIdentity(key, certificate)
    }

    @RequiresApi(Build.VERSION_CODES.Q)
    private fun create(): DeviceIdentity {
        val generator = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, ANDROID_KEYSTORE)
        try {
            generator.initialize(spec(strongBox = true))
            generator.generateKeyPair()
        } catch (e: java.security.ProviderException) {
            // Pas de StrongBox (StrongBoxUnavailableException), ou StrongBox défaillant : environnement sécurisé.
            DiagnosticLog.i("identité", "StrongBox indisponible (${e.javaClass.simpleName}) : clé dans l'environnement sécurisé")
            generator.initialize(spec(strongBox = false))
            generator.generateKeyPair()
        }
        DiagnosticLog.i("identité", "identité wolagent/2 de ce téléphone créée")
        return load() ?: throw GeneralSecurityException("identité introuvable après sa création")
    }

    @RequiresApi(Build.VERSION_CODES.Q)
    private fun spec(strongBox: Boolean): KeyGenParameterSpec =
        KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN)
            .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
            // TLS fait signer par la clé le condensé qu'il a calculé lui-même (NONE), ou le message (SHA-256).
            .setDigests(KeyProperties.DIGEST_NONE, KeyProperties.DIGEST_SHA256)
            .setCertificateSubject(X500Principal("CN=wolagent"))
            .setCertificateSerialNumber(BigInteger(127, SecureRandom()).setBit(126))
            .setIsStrongBoxBacked(strongBox)
            .build()

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val ALIAS = "wolagent2-identity"
    }
}
