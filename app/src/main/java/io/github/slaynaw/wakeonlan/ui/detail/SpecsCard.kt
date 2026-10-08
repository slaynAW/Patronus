package io.github.slaynaw.wakeonlan.ui.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.agent.AgentSpecs
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.SectionLabel
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.eventTimeText
import io.github.slaynaw.wakeonlan.ui.common.formatBytes
import io.github.slaynaw.wakeonlan.ui.devices.DeviceItem
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import java.text.NumberFormat
import java.util.Locale

/** Version de l'agent qui donne la fiche du PC. */
private const val SPECS_AGENT = "1.10.0"

/**
 * Fiche du PC (agent 1.10.0) : processeur, mémoire, cartes graphiques, carte mère et système, gardée
 * sur le téléphone pour rester visible PC éteint.
 */
@Composable
internal fun SpecsCard(item: DeviceItem, now: Long) {
    val stored = item.specs
    val agent = item.status.agent?.takeIf { item.status.state == PowerState.ONLINE }
    if (stored == null && agent == null) return
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        SectionLabel(stringResource(R.string.specs_title))
        WolCard {
            if (stored == null) {
                Text(
                    if (agent != null && olderThan(agent.version, SPECS_AGENT)) {
                        stringResource(R.string.specs_old_agent, "v$SPECS_AGENT")
                    } else {
                        stringResource(R.string.specs_loading)
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = WolPalette.Text2,
                    modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
                )
            } else {
                SelectionContainer {
                    Column {
                        specRows(stored.specs).forEachIndexed { i, row ->
                            if (i > 0) RowDivider()
                            SpecRow(row)
                        }
                    }
                }
            }
        }
        if (stored != null) {
            Text(
                stringResource(R.string.specs_read, eventTimeText(stored.fetched, now)),
                style = MaterialTheme.typography.labelSmall,
                color = WolPalette.Text3,
                modifier = Modifier.padding(horizontal = 2.dp),
            )
        }
    }
}

/** Une ligne de la fiche : intitulé, valeur principale et précisions. */
private data class Spec(val label: String, val value: String, val details: List<String>)

@Composable
private fun SpecRow(spec: Spec) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 10.dp)) {
        Text(spec.label, style = MaterialTheme.typography.labelSmall, color = WolPalette.Text3)
        Text(spec.value, style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold), color = WolPalette.Text)
        spec.details.forEach { Text(it, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text3) }
    }
}

/** Taille ronde sans décimale inutile (« 32 Go », « 15,7 Go »). */
private fun sizeText(bytes: Long): String = formatBytes(bytes).replace(Regex(",0(?= )"), "")

private fun join(vararg parts: String): String = parts.filter { it.isNotEmpty() }.joinToString(" · ")

@Composable
private fun specRows(specs: AgentSpecs): List<Spec> {
    val resources = LocalResources.current
    val rows = mutableListOf<Spec>()
    if (specs.model.isNotEmpty()) rows += Spec(stringResource(R.string.specs_model), specs.model, emptyList())
    specs.cpu?.takeIf { it.name.isNotEmpty() || it.threads > 0 }?.let { c ->
        val cores = when {
            c.cores > 0 && c.threads > 0 -> stringResource(R.string.specs_cores_threads, c.cores, c.threads)
            c.threads > 0 -> stringResource(R.string.specs_threads, c.threads)
            else -> ""
        }
        val ghz = if (c.mhz > 0) {
            val format = NumberFormat.getNumberInstance(Locale.FRANCE).apply { maximumFractionDigits = 1 }
            stringResource(R.string.specs_ghz, format.format(c.mhz / 1000.0))
        } else {
            ""
        }
        val count = if (c.count > 1) stringResource(R.string.specs_cpu_count, c.count) else ""
        rows += Spec(stringResource(R.string.specs_cpu), c.name.ifEmpty { "—" }, listOf(join(count, cores, ghz)).filter { it.isNotEmpty() })
    }
    specs.memory?.takeIf { it.total > 0 }?.let { m ->
        val kinds = m.modules.map { it.kind }.filter { it.isNotEmpty() }.distinct()
        val count = if (m.modules.isEmpty()) {
            ""
        } else {
            val modules = resources.getQuantityString(R.plurals.specs_modules, m.modules.size, m.modules.size)
            if (m.slots > 0) stringResource(R.string.specs_modules_slots, modules, m.slots) else modules
        }
        // Type et vitesse communs : déjà sur la première ligne.
        val lines = m.modules.map { x ->
            join(x.slot, sizeText(x.size), if (kinds.size == 1) "" else x.kind, listOf(x.maker, x.part).filter { it.isNotEmpty() }.joinToString(" "))
        }
        rows += Spec(stringResource(R.string.specs_ram), join(sizeText(m.total), kinds.singleOrNull().orEmpty()), (listOf(count) + lines).filter { it.isNotEmpty() })
    }
    for (g in specs.gpus) {
        val details = join(
            if (g.integrated) stringResource(R.string.specs_integrated) else "",
            if (g.vram > 0) stringResource(R.string.specs_vram, sizeText(g.vram)) else "",
            if (g.driver.isNotEmpty()) stringResource(R.string.specs_driver, g.driver) else "",
        )
        rows += Spec(stringResource(R.string.specs_gpu), g.name.ifEmpty { "—" }, listOf(details).filter { it.isNotEmpty() })
    }
    specs.board?.takeIf { it.maker.isNotEmpty() || it.model.isNotEmpty() || it.bios.isNotEmpty() }?.let { b ->
        val date = Regex("""^(\d{4})-(\d{2})-(\d{2})$""").find(b.biosDate)?.destructured?.let { (y, mo, d) -> "$d/$mo/$y" }
        val bios = when {
            b.bios.isEmpty() -> ""
            date != null -> stringResource(R.string.specs_bios_date, b.bios, date)
            else -> stringResource(R.string.specs_bios, b.bios)
        }
        val name = listOf(b.maker, b.model).filter { it.isNotEmpty() }.joinToString(" ").ifEmpty { "—" }
        rows += Spec(stringResource(R.string.specs_board), name, listOf(bios).filter { it.isNotEmpty() })
    }
    specs.os?.takeIf { it.name.isNotEmpty() }?.let { o ->
        rows += Spec(stringResource(R.string.specs_os), o.name, listOf(o.version).filter { it.isNotEmpty() })
    }
    return rows
}

/** Numéros d'une version (« 1.9.0 » : 1, 9, 0), ou `null` si elle est illisible (« dev »). */
private fun versionParts(version: String): List<Int>? {
    val parts = version.trim().removePrefix("v").split('.').map { it.takeWhile(Char::isDigit).toIntOrNull() }
    return if (parts.any { it == null }) null else parts.filterNotNull()
}

/** Vrai si la version [version] (« 1.9.0 ») précède [reference] ; une version illisible ne la précède pas. */
private fun olderThan(version: String, reference: String): Boolean {
    val a = versionParts(version) ?: return false
    val b = versionParts(reference) ?: return false
    for (i in 0 until maxOf(a.size, b.size)) {
        val x = a.getOrElse(i) { 0 }
        val y = b.getOrElse(i) { 0 }
        if (x != y) return x < y
    }
    return false
}
