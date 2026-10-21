package io.github.slaynaw.wakeonlan.security

import android.app.KeyguardManager
import android.content.Context
import android.os.SystemClock
import androidx.core.content.edit
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** État du verrouillage pour l'interface. */
data class AppLockState(
    /** Empreinte, visage ou code du téléphone demandés avant les actions sensibles. */
    val actions: Boolean = false,
    /** Empreinte, visage ou code demandés à l'ouverture de l'application. */
    val open: Boolean = false,
    /** Le téléphone a un verrouillage d'écran (code, schéma ou mot de passe) : sans lui, rien ne peut être demandé. */
    val available: Boolean = false,
    /** L'application attend d'être déverrouillée (verrouillage à l'ouverture). */
    val locked: Boolean = false,
)

/** Réglages du verrouillage, propres à ce téléphone : jamais exportés ni sauvegardés. */
interface AppLockStore {
    fun get(key: String): Boolean

    fun set(key: String, value: Boolean)
}

/**
 * Verrouillage facultatif de l'application (docs/SECURITE.md), contre une personne qui emprunte le téléphone déjà
 * déverrouillé : empreinte, visage ou code du téléphone avant les actions sensibles (extinction, administration des
 * appareils, partage, export, clé d'agent affichée) et/ou à l'ouverture. Après une authentification, les actions
 * suivantes ne redemandent rien pendant [GRACE_MS], tant que l'application reste au premier plan.
 *
 * C'est un verrou d'interface : il ne remplace ni le verrouillage du téléphone, ni le retrait d'un appareil perdu.
 */
class AppLock(
    private val store: AppLockStore,
    /** Le téléphone a un verrouillage d'écran. */
    private val secure: () -> Boolean,
    /** Horloge monotone, en millisecondes. */
    private val clock: () -> Long = SystemClock::elapsedRealtime,
) {
    private var lastAuth: Long? = null
    private var stoppedAt: Long? = null

    private val _state = MutableStateFlow(
        run {
            val open = store.get(KEY_OPEN)
            val available = secure()
            AppLockState(actions = store.get(KEY_ACTIONS), open = open, available = available, locked = open && available)
        },
    )
    val state: StateFlow<AppLockState> = _state.asStateFlow()

    /** Une action sensible doit être confirmée maintenant. */
    fun needsAuth(): Boolean {
        val s = _state.value
        if (!s.actions || !s.available) return false
        val last = lastAuth ?: return true
        return clock() - last > GRACE_MS
    }

    /** Authentification réussie : application déverrouillée, et délai de grâce pour les actions sensibles. */
    fun onAuthenticated() {
        lastAuth = clock()
        _state.update { it.copy(locked = false) }
    }

    fun setActions(enabled: Boolean) {
        store.set(KEY_ACTIONS, enabled)
        _state.update { it.copy(actions = enabled) }
        DiagnosticLog.i(AREA, "confirmation des actions sensibles ${if (enabled) "activée" else "désactivée"}")
    }

    fun setOpen(enabled: Boolean) {
        store.set(KEY_OPEN, enabled)
        _state.update { it.copy(open = enabled, locked = false) }
        DiagnosticLog.i(AREA, "verrouillage à l'ouverture ${if (enabled) "activé" else "désactivé"}")
    }

    /**
     * L'application revient au premier plan : verrouillée si elle est restée plus de [LOCK_AFTER_MS] en arrière-plan.
     * Une authentification réussie entre-temps (écran du code d'Android affiché par-dessus) compte. Le verrouillage
     * d'écran du téléphone a pu être ajouté ou retiré.
     */
    fun onStart() {
        val available = secure()
        val stopped = stoppedAt
        stoppedAt = null
        // onStop efface lastAuth : s'il est connu, l'authentification a eu lieu pendant l'arrière-plan.
        val relock = stopped != null && clock() - stopped > LOCK_AFTER_MS && lastAuth == null
        _state.update { s -> s.copy(available = available, locked = s.open && available && (s.locked || relock)) }
    }

    /** L'application passe en arrière-plan : le délai de grâce prend fin. */
    fun onStop() {
        stoppedAt = clock()
        lastAuth = null
    }

    companion object {
        private const val AREA = "sécurité"
        private const val KEY_ACTIONS = "lock_actions"
        private const val KEY_OPEN = "lock_open"

        /** Après une authentification, les actions sensibles suivantes ne redemandent rien pendant une minute. */
        const val GRACE_MS = 60_000L

        /** Verrouillage à l'ouverture : de nouveau après une minute en arrière-plan. */
        const val LOCK_AFTER_MS = 60_000L

        /** Verrouillage de ce téléphone : préférences privées de l'application, exclues des sauvegardes d'Android. */
        fun create(context: Context): AppLock {
            val app = context.applicationContext
            val prefs = app.getSharedPreferences("security", Context.MODE_PRIVATE)
            val keyguard = app.getSystemService(KeyguardManager::class.java)
            val store = object : AppLockStore {
                override fun get(key: String): Boolean = prefs.getBoolean(key, false)

                override fun set(key: String, value: Boolean) {
                    prefs.edit { putBoolean(key, value) }
                }
            }
            return AppLock(store, secure = { keyguard?.isDeviceSecure == true })
        }
    }
}
