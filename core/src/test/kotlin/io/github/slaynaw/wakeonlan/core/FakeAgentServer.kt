package io.github.slaynaw.wakeonlan.core

import io.github.slaynaw.wakeonlan.core.agent.AgentProtocol
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import java.net.InetAddress
import java.net.ServerSocket
import java.security.SecureRandom
import kotlin.concurrent.thread

/**
 * Mini-agent écrit en Kotlin pour tester le client sans dépendre de l'agent Go.
 * [behavior] permet de simuler des agents défectueux ou malveillants.
 */
class FakeAgentServer(
    private val key: ByteArray,
    private val behavior: Behavior = Behavior.NORMAL,
) : AutoCloseable {
    enum class Behavior { NORMAL, BAD_RESPONSE_MAC, REJECT, WRONG_PROTO, HANG, RATE_LIMITED }

    private val server = ServerSocket(0, 50, InetAddress.getLoopbackAddress())
    val port: Int get() = server.localPort
    val commands = mutableListOf<String>()

    init {
        thread(isDaemon = true) {
            while (!server.isClosed) {
                val socket = runCatching { server.accept() }.getOrNull() ?: break
                thread(isDaemon = true) { socket.use { handle(it) } }
            }
        }
    }

    private fun handle(socket: java.net.Socket) {
        val reader = socket.getInputStream().bufferedReader()
        val writer = socket.getOutputStream().bufferedWriter()
        if (behavior == Behavior.HANG) {
            Thread.sleep(10_000)
            return
        }
        if (behavior == Behavior.RATE_LIMITED) {
            writer.write("""{"error":"rate_limited"}""" + "\n")
            writer.flush()
            return
        }
        val nonce = AgentProtocol.encodeNonce(ByteArray(32).also(SecureRandom()::nextBytes))
        val proto = if (behavior == Behavior.WRONG_PROTO) "wolagent/99" else AgentProtocol.PROTO
        writer.write("""{"proto":"$proto","nonce":"$nonce"}""" + "\n")
        writer.flush()

        val request = Json.parseToJsonElement(reader.readLine() ?: return).jsonObject
        val cnonce = request.str("cnonce")
        val body = request.str("body")
        val expected = AgentProtocol.requestMac(key, nonce, cnonce, body)
        if (!AgentProtocol.macEquals(expected, request.str("mac"))) {
            writer.write("""{"error":"unauthorized"}""" + "\n")
            writer.flush()
            return
        }
        val cmd = Json.parseToJsonElement(body).jsonObject.str("cmd")
        synchronized(commands) { commands += cmd }
        val ok = behavior != Behavior.REJECT
        val responseBody = buildJsonObject {
            put("ok", ok)
            put("code", if (ok) "ok" else "error")
            put("message", if (ok) "OK $cmd" else "refusé")
            put("hostname", "PC-TEST")
            put("os", "linux")
            put("arch", "amd64")
            put("version", "1.0.0")
            put("uptime", 3600)
        }.toString()
        var mac = AgentProtocol.responseMac(key, nonce, cnonce, responseBody)
        if (behavior == Behavior.BAD_RESPONSE_MAC) mac = AgentProtocol.responseMac(ByteArray(32), nonce, cnonce, responseBody)
        val response = buildJsonObject {
            put("body", responseBody)
            put("mac", mac)
        }
        writer.write(response.toString() + "\n")
        writer.flush()
    }

    private fun JsonObject.str(name: String) = (this[name] as JsonPrimitive).jsonPrimitive.content

    override fun close() = server.close()
}
