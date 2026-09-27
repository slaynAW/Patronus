package io.github.slaynaw.wakeonlan.core

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.io.File

@Serializable
data class ProtocolVector(
    val name: String,
    val key: String,
    val nonce: String,
    val cnonce: String,
    val requestBody: String,
    val requestMac: String,
    val responseBody: String,
    val responseMac: String,
)

@Serializable
data class MagicPacketVector(val mac: String, val secureOn: String?, val packetHex: String)

@Serializable
data class TestVectors(val protocol: String, val vectors: List<ProtocolVector>, val magicPackets: List<MagicPacketVector>)

/** Vecteurs partagés avec l'agent Go : protocol/test-vectors.json. */
object SharedVectors {
    private val json = Json { ignoreUnknownKeys = true }

    val value: TestVectors by lazy {
        val dir = System.getProperty("wol.protocolDir") ?: "../protocol"
        json.decodeFromString(File(dir, "test-vectors.json").readText())
    }
}

fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it.toInt() and 0xFF) }
