package io.github.slaynaw.wakeonlan.security

import android.app.Application
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Verrouillage facultatif : quand l'empreinte, le visage ou le code sont demandés (Robolectric pour le journal). */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class AppLockTest {
    private val stored = mutableMapOf<String, Boolean>()
    private val store = object : AppLockStore {
        override fun get(key: String): Boolean = stored[key] ?: false

        override fun set(key: String, value: Boolean) {
            stored[key] = value
        }
    }
    private var secure = true
    private var now = 1_000_000L

    private fun lock() = AppLock(store, secure = { secure }, clock = { now })

    @Test
    fun desactiveParDefaut() {
        val lock = lock()
        assertEquals(AppLockState(available = true), lock.state.value)
        assertFalse(lock.needsAuth())
    }

    @Test
    fun actionsSensiblesAvecDelaiDeGrace() {
        val lock = lock()
        lock.setActions(true)
        assertTrue(lock.needsAuth())
        lock.onAuthenticated()
        assertFalse(lock.needsAuth())
        now += AppLock.GRACE_MS
        assertFalse(lock.needsAuth())
        now += 1
        assertTrue(lock.needsAuth())
    }

    @Test
    fun arrierePlanFinitLeDelaiDeGrace() {
        val lock = lock()
        lock.setActions(true)
        lock.onAuthenticated()
        lock.onStop()
        now += 1_000
        lock.onStart()
        assertTrue(lock.needsAuth())
    }

    @Test
    fun sansVerrouillageDEcranRienNEstDemande() {
        secure = false
        val lock = lock()
        lock.setActions(true)
        lock.setOpen(true)
        assertFalse(lock.needsAuth())
        lock.onStop()
        now += AppLock.LOCK_AFTER_MS + 1
        lock.onStart()
        assertFalse(lock.state.value.locked)
        assertFalse(lock.state.value.available)
    }

    @Test
    fun verrouilleALOuverture() {
        stored["lock_open"] = true
        val lock = lock()
        assertTrue(lock.state.value.locked)
        lock.onStart()
        assertTrue(lock.state.value.locked)
        lock.onAuthenticated()
        assertFalse(lock.state.value.locked)
    }

    @Test
    fun reverrouilleApresUneMinuteEnArrierePlan() {
        stored["lock_open"] = true
        val lock = lock()
        lock.onAuthenticated()

        lock.onStop()
        now += AppLock.LOCK_AFTER_MS
        lock.onStart()
        assertFalse(lock.state.value.locked)

        lock.onStop()
        now += AppLock.LOCK_AFTER_MS + 1
        lock.onStart()
        assertTrue(lock.state.value.locked)
    }

    @Test
    fun authentificationPendantLArrierePlanCompte() {
        // Android 9 et avant : l'écran du code d'Android passe devant l'application, qui s'arrête.
        stored["lock_open"] = true
        val lock = lock()
        lock.onAuthenticated()
        lock.onStop()
        now += AppLock.LOCK_AFTER_MS + 1
        lock.onAuthenticated()
        lock.onStart()
        assertFalse(lock.state.value.locked)
    }

    @Test
    fun verrouillageDEcranAjouteEnArrierePlan() {
        secure = false
        stored["lock_open"] = true
        val lock = lock()
        assertFalse(lock.state.value.locked)
        lock.onStop()
        secure = true
        lock.onStart()
        // Toujours ouverte : l'application était déjà utilisée sans verrouillage, et vient de revenir.
        assertFalse(lock.state.value.locked)
        assertTrue(lock.state.value.available)
        lock.onStop()
        now += AppLock.LOCK_AFTER_MS + 1
        lock.onStart()
        assertTrue(lock.state.value.locked)
    }

    @Test
    fun reglagesEnregistres() {
        val lock = lock()
        lock.setActions(true)
        lock.setOpen(true)
        assertEquals(mapOf("lock_actions" to true, "lock_open" to true), stored)
        assertFalse("l'activer ne verrouille pas aussitôt", lock.state.value.locked)

        val next = lock()
        assertTrue(next.state.value.actions)
        assertTrue(next.state.value.locked)

        next.setOpen(false)
        assertFalse(next.state.value.locked)
        assertEquals(false, stored["lock_open"])
    }
}
