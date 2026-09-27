package io.github.slaynaw.wakeonlan.core.net

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import java.io.Closeable
import java.net.InetSocketAddress
import java.net.Socket
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * Exécute une opération bloquante sur une ressource, en la fermant si la coroutine est annulée :
 * c'est le seul moyen fiable d'interrompre un `connect()` ou un `read()` bloquant.
 */
suspend fun <C : Closeable, R> C.useCancellable(block: (C) -> R): R = withContext(Dispatchers.IO) {
    val resource = this@useCancellable
    try {
        suspendCancellableCoroutine { cont ->
            cont.invokeOnCancellation { runCatching { resource.close() } }
            try {
                cont.resume(block(resource))
            } catch (t: Throwable) {
                if (cont.isActive) cont.resumeWithException(t)
            }
        }
    } finally {
        runCatching { resource.close() }
    }
}

/** Ouvre une connexion TCP (attachée au réseau local) avec un délai maximal. */
fun Socket.connectBound(binder: NetworkBinder, address: InetSocketAddress, timeoutMs: Int) {
    binder.bind(this)
    tcpNoDelay = true
    connect(address, timeoutMs)
}

private val detachedIoScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

/**
 * Exécute un appel bloquant NON interruptible (résolution DNS, ping ICMP) sur un thread d'E/S.
 * L'attente, elle, reste annulable : un appelant pressé n'est jamais bloqué par l'appel.
 */
suspend fun <T> awaitBlocking(block: () -> T): T = detachedIoScope.async { block() }.await()
