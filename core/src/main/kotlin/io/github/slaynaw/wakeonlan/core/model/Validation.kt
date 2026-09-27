package io.github.slaynaw.wakeonlan.core.model

import io.github.slaynaw.wakeonlan.core.agent.AgentKey

/** Champ d'un terminal, pour rattacher une erreur de validation au bon champ du formulaire. */
enum class DeviceField { NAME, MAC, HOST, BROADCAST, WOL_PORT, SECURE_ON, PROBE_PORTS, AGENT_PORT, AGENT_KEY }

data class ValidationError(val field: DeviceField, val message: String)

/**
 * Règles de validation d'un terminal. Utilisées par le formulaire de saisie ET à l'import
 * d'une configuration, pour ne jamais faire confiance à un fichier externe.
 */
object DeviceValidator {
    const val MAX_NAME_LENGTH = 64
    const val MAX_HOST_LENGTH = 253
    const val MAX_PROBE_PORTS = 8

    private val HOSTNAME_LABEL = Regex("^(?!-)[A-Za-z0-9-]{1,63}(?<!-)$")
    private val IPV4 = Regex("^((25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)\\.){3}(25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)$")
    private val IPV6_CHARS = Regex("^[0-9A-Fa-f:.]+(%[A-Za-z0-9_.\\-]+)?$")
    private val SECURE_ON = Regex("^[0-9A-Fa-f]{2}([:-]?[0-9A-Fa-f]{2}){5}$")

    fun validate(device: Device): List<ValidationError> = buildList {
        val name = device.name.trim()
        if (name.isEmpty()) add(ValidationError(DeviceField.NAME, "Le nom est obligatoire"))
        if (name.length > MAX_NAME_LENGTH) add(ValidationError(DeviceField.NAME, "Nom trop long (max $MAX_NAME_LENGTH)"))
        if (name.any { it.isISOControl() }) add(ValidationError(DeviceField.NAME, "Caractères interdits dans le nom"))

        if (device.host.isNotEmpty() && !isValidHost(device.host)) {
            add(ValidationError(DeviceField.HOST, "Adresse IP ou nom d'hôte invalide"))
        }
        device.broadcastAddress?.let {
            if (!isIpv4(it)) add(ValidationError(DeviceField.BROADCAST, "Adresse de diffusion IPv4 invalide"))
        }
        if (!isValidPort(device.wolPort)) add(ValidationError(DeviceField.WOL_PORT, "Port invalide (1-65535)"))
        device.secureOnPassword?.let {
            if (!SECURE_ON.matches(it)) {
                add(ValidationError(DeviceField.SECURE_ON, "Mot de passe SecureOn : 6 octets hexadécimaux"))
            }
        }
        if (device.probePorts.size > MAX_PROBE_PORTS || device.probePorts.any { !isValidPort(it) }) {
            add(ValidationError(DeviceField.PROBE_PORTS, "Ports de détection invalides (max $MAX_PROBE_PORTS)"))
        }
        device.agent?.let { agent ->
            if (!isValidPort(agent.port)) add(ValidationError(DeviceField.AGENT_PORT, "Port invalide (1-65535)"))
            if (agent.hasKey && AgentKey.decodeOrNull(agent.key) == null) {
                add(ValidationError(DeviceField.AGENT_KEY, "Clé d'agent invalide"))
            }
            if (!device.hasHost) add(ValidationError(DeviceField.HOST, "L'adresse du PC est nécessaire pour l'agent"))
        }
    }

    fun isValidPort(port: Int): Boolean = port in 1..65535

    fun isIpv4(value: String): Boolean = IPV4.matches(value)

    /** IPv4, IPv6 littérale ou nom d'hôte RFC 1123 (ex. `pc-bureau.local`). */
    fun isValidHost(value: String): Boolean {
        if (value.isEmpty() || value.length > MAX_HOST_LENGTH || value != value.trim()) return false
        if (isIpv4(value)) return true
        if (value.contains(':')) return IPV6_CHARS.matches(value) && value.count { it == ':' } in 2..7
        // Un nom composé uniquement de chiffres et de points est une IPv4 mal formée.
        if (value.all { it.isDigit() || it == '.' }) return false
        return value.removeSuffix(".").split('.').all { HOSTNAME_LABEL.matches(it) }
    }

    /** Convertit un mot de passe SecureOn validé en 6 octets. */
    fun secureOnBytes(value: String): ByteArray {
        require(SECURE_ON.matches(value)) { "Mot de passe SecureOn invalide" }
        val hex = value.replace(Regex("[:-]"), "")
        return ByteArray(6) { i -> hex.substring(i * 2, i * 2 + 2).toInt(16).toByte() }
    }
}
