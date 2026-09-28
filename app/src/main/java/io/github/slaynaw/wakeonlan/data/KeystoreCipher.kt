package io.github.slaynaw.wakeonlan.data

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.nio.ByteBuffer
import java.security.GeneralSecurityException
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Chiffrement AES-256-GCM avec une clé générée et conservée dans le Keystore Android
 * (matériel sécurisé du téléphone). La clé n'est jamais exportable, même avec un accès root.
 *
 * Format produit : `[version=1][taille IV][IV][texte chiffré + tag]`. [aad] lie les données à leur
 * usage (configuration, historique) : un fichier ne peut pas être substitué à l'autre.
 */
class KeystoreCipher(private val alias: String = DEFAULT_ALIAS, aad: String = DEFAULT_AAD) {

    private val aad = aad.toByteArray()

    private val keyStore: KeyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }

    @Synchronized
    private fun key(): SecretKey =
        (keyStore.getKey(alias, null) as? SecretKey) ?: KeyGenerator.getInstance(
            KeyProperties.KEY_ALGORITHM_AES,
            ANDROID_KEYSTORE,
        ).run {
            init(
                KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setKeySize(256)
                    .setRandomizedEncryptionRequired(true)
                    .build(),
            )
            generateKey()
        }

    fun encrypt(plain: ByteArray): ByteArray {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        cipher.updateAAD(aad)
        val iv = cipher.iv
        val encrypted = cipher.doFinal(plain)
        return ByteBuffer.allocate(2 + iv.size + encrypted.size)
            .put(FORMAT_VERSION)
            .put(iv.size.toByte())
            .put(iv)
            .put(encrypted)
            .array()
    }

    fun decrypt(data: ByteArray): ByteArray {
        if (data.size < 2 || data[0] != FORMAT_VERSION) throw GeneralSecurityException("Format inconnu")
        val ivSize = data[1].toInt()
        if (ivSize !in 12..16 || data.size < 2 + ivSize + 16) throw GeneralSecurityException("Données tronquées")
        val iv = data.copyOfRange(2, 2 + ivSize)
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, iv))
        cipher.updateAAD(aad)
        return cipher.doFinal(data, 2 + ivSize, data.size - 2 - ivSize)
    }

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val DEFAULT_ALIAS = "wakeonlan-config-v1"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val FORMAT_VERSION: Byte = 1
        const val DEFAULT_AAD = "wakeonlan/config"
    }
}
