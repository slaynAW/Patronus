package io.github.slaynaw.wakeonlan.core.model

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder

/**
 * Adresse matérielle (MAC) sur 6 octets, immuable.
 *
 * Formats acceptés en entrée : `AA:BB:CC:DD:EE:FF`, `AA-BB-CC-DD-EE-FF`,
 * `AABB.CCDD.EEFF` (Cisco) et `AABBCCDDEEFF`. La casse est ignorée.
 * La forme canonique (sortie) est `AA:BB:CC:DD:EE:FF`.
 */
@Serializable(with = MacAddressSerializer::class)
class MacAddress private constructor(private val bytes: ByteArray) {

    /** Copie défensive des 6 octets. */
    fun toByteArray(): ByteArray = bytes.copyOf()

    override fun toString(): String = bytes.joinToString(":") { "%02X".format(it.toInt() and 0xFF) }

    override fun equals(other: Any?): Boolean = other is MacAddress && bytes.contentEquals(other.bytes)

    override fun hashCode(): Int = bytes.contentHashCode()

    companion object {
        const val LENGTH = 6

        private val SEPARATORS = Regex("[:\\-.\\s]")
        private val HEX12 = Regex("^[0-9a-fA-F]{12}$")
        private val GROUPED = Regex("^[0-9a-fA-F]{2}([:-])[0-9a-fA-F]{2}(\\1[0-9a-fA-F]{2}){4}$")
        private val CISCO = Regex("^[0-9a-fA-F]{4}\\.[0-9a-fA-F]{4}\\.[0-9a-fA-F]{4}$")

        /**
         * Analyse une adresse MAC.
         * @throws IllegalArgumentException si le format est invalide ou si l'adresse ne peut pas
         * désigner une carte réseau (tout à zéro, broadcast).
         */
        fun parse(input: String): MacAddress {
            val trimmed = input.trim()
            require(GROUPED.matches(trimmed) || CISCO.matches(trimmed) || HEX12.matches(trimmed)) {
                "Adresse MAC invalide : « $input »"
            }
            val hex = trimmed.replace(SEPARATORS, "")
            val bytes = ByteArray(LENGTH) { i -> hex.substring(i * 2, i * 2 + 2).toInt(16).toByte() }
            require(bytes.any { it != 0.toByte() }) { "L'adresse MAC ne peut pas être 00:00:00:00:00:00" }
            require(bytes.any { it != 0xFF.toByte() }) { "L'adresse MAC ne peut pas être l'adresse de broadcast" }
            return MacAddress(bytes)
        }

        fun parseOrNull(input: String): MacAddress? = runCatching { parse(input) }.getOrNull()

        fun fromBytes(bytes: ByteArray): MacAddress {
            require(bytes.size == LENGTH) { "Une adresse MAC fait $LENGTH octets" }
            return parse(bytes.joinToString(":") { "%02X".format(it.toInt() and 0xFF) })
        }
    }
}

internal object MacAddressSerializer : KSerializer<MacAddress> {
    override val descriptor: SerialDescriptor =
        PrimitiveSerialDescriptor("io.github.slaynaw.wakeonlan.MacAddress", PrimitiveKind.STRING)

    override fun serialize(encoder: Encoder, value: MacAddress) = encoder.encodeString(value.toString())

    override fun deserialize(decoder: Decoder): MacAddress = MacAddress.parse(decoder.decodeString())
}
