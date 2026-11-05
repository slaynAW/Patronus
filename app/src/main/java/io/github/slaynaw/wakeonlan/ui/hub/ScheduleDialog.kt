package io.github.slaynaw.wakeonlan.ui.hub

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.hub.HubPc
import io.github.slaynaw.wakeonlan.core.hub.HubPcSchedule
import io.github.slaynaw.wakeonlan.core.hub.HubProtocol
import io.github.slaynaw.wakeonlan.core.hub.HubSlot
import io.github.slaynaw.wakeonlan.ui.admin.RoleChoice
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.theme.MonoStyle
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

private val WEEKDAYS = listOf(1, 2, 3, 4, 5)
private val WEEKEND = listOf(6, 7)
private val MINUTES = (0..55 step 5).toList()

private fun two(n: Int): String = n.toString().padStart(2, '0')

/** Résumé des jours d'une programmation : « du lundi au vendredi », « lun., mer. ». */
@Composable
internal fun daysText(days: List<Int>): String = when (days.sorted()) {
    (1..7).toList() -> stringResource(R.string.hub_schedule_days_all)
    WEEKDAYS -> stringResource(R.string.hub_schedule_days_weekdays)
    WEEKEND -> stringResource(R.string.hub_schedule_days_weekend)
    else -> days.sorted().map { dayLong(it) }.joinToString(", ")
}

@Composable
private fun dayShort(day: Int): String = stringResource(
    when (day) {
        1 -> R.string.day_1
        2 -> R.string.day_2
        3 -> R.string.day_3
        4 -> R.string.day_4
        5 -> R.string.day_5
        6 -> R.string.day_6
        else -> R.string.day_7
    },
)

@Composable
private fun dayLong(day: Int): String = stringResource(
    when (day) {
        1 -> R.string.day_long_1
        2 -> R.string.day_long_2
        3 -> R.string.day_long_3
        4 -> R.string.day_long_4
        5 -> R.string.day_long_5
        6 -> R.string.day_long_6
        else -> R.string.day_long_7
    },
)

/** Résumé de la programmation active d'un PC (vide : aucune). */
@Composable
internal fun scheduleText(schedule: HubPcSchedule?): String {
    val parts = mutableListOf<String>()
    schedule?.wake?.takeIf { it.on }?.let { parts += stringResource(R.string.hub_schedule_wake_at, daysText(it.days), it.at) }
    schedule?.shutdown?.takeIf { it.on }?.let { parts += stringResource(R.string.hub_schedule_shutdown_at, daysText(it.days), it.at) }
    return parts.joinToString(" · ")
}

/** Heure d'une occurrence, dans le fuseau du hub (celui des heures choisies). */
private fun hubTime(seconds: Long, timezone: String): String {
    val zone = runCatching { ZoneId.of(timezone) }.getOrDefault(ZoneId.systemDefault())
    return DateTimeFormatter.ofPattern("d MMM yyyy 'à' HH:mm", Locale.FRENCH).format(Instant.ofEpochSecond(seconds).atZone(zone))
}

/** Une partie de la programmation en cours de modification. */
private class SlotState(slot: HubSlot?, defaultAt: String) {
    var on by mutableStateOf(slot?.on == true)
    val days = mutableStateListOf<Int>().apply { addAll(slot?.days ?: WEEKDAYS) }
    var hour by mutableStateOf((slot?.at ?: defaultAt).take(2).toIntOrNull() ?: 0)
    var minute by mutableStateOf((slot?.at ?: defaultAt).takeLast(2).toIntOrNull() ?: 0)

    /**
     * Ce qu'il faut envoyer au hub : partie que ce téléphone ne peut pas changer ([enabled] faux) gardée telle quelle ;
     * désactivée, gardée (sans effet) si elle existait, absente sinon.
     */
    fun value(original: HubSlot?, enabled: Boolean): HubSlot? = when {
        !enabled -> original
        !on -> original?.copy(on = false, next = 0)
        else -> HubSlot(on = true, days = days.sorted(), at = "${two(hour)}:${two(minute)}")
    }
}

/**
 * Programmation d'un PC du hub (docs/PROTOCOLE-HUB.md, §6.7) : réveil (appareil avec MAC) et extinction si inactif (PC
 * avec agent que ce téléphone administre, [canShutdown]), jours et heure dans le fuseau [timezone] du hub.
 */
@Composable
internal fun ScheduleDialog(
    pc: HubPc,
    current: HubPcSchedule?,
    timezone: String,
    canShutdown: Boolean,
    onSave: (wake: HubSlot?, shutdown: HubSlot?) -> Unit,
    onDismiss: () -> Unit,
) {
    val canWake = pc.mac.isNotEmpty()
    val wake = remember { SlotState(current?.wake, "07:30") }
    val shutdown = remember { SlotState(current?.shutdown, "23:00") }
    var problem by remember { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(WolIcons.Clock, contentDescription = null, tint = WolPalette.Blue) },
        title = { Text(stringResource(R.string.hub_schedule_title, pc.name)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    stringResource(if (pc.agent.isNotEmpty()) R.string.hub_schedule_text else R.string.hub_schedule_text_device, timezone),
                    style = MaterialTheme.typography.bodyMedium,
                )
                SlotEditor(
                    title = stringResource(R.string.hub_schedule_wake),
                    state = wake,
                    original = current?.wake,
                    enabled = canWake,
                    note = if (canWake) null else stringResource(R.string.hub_schedule_wake_no_mac),
                    timezone = timezone,
                )
                if (pc.agent.isNotEmpty()) {
                    HorizontalDivider(color = WolPalette.Line)
                    SlotEditor(
                        title = stringResource(R.string.hub_schedule_shutdown),
                        state = shutdown,
                        original = current?.shutdown,
                        enabled = canShutdown,
                        note = stringResource(if (canShutdown) R.string.hub_schedule_shutdown_help else R.string.hub_schedule_shutdown_no_admin),
                        timezone = timezone,
                    )
                }
                if (problem) {
                    Text(stringResource(R.string.hub_schedule_no_day), style = MaterialTheme.typography.bodySmall, color = WolPalette.DangerText)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val w = wake.value(current?.wake, canWake)
                val s = if (pc.agent.isNotEmpty()) shutdown.value(current?.shutdown, canShutdown) else current?.shutdown
                problem = listOfNotNull(w, s).any { it.on && !HubProtocol.validSlot(it) }
                if (!problem) onSave(w, s)
            }) { Text(stringResource(R.string.save)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun SlotEditor(title: String, state: SlotState, original: HubSlot?, enabled: Boolean, note: String?, timezone: String) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Switch(
                checked = state.on,
                onCheckedChange = { state.on = it },
                enabled = enabled,
                colors = SwitchDefaults.colors(
                    checkedThumbColor = Color.White,
                    checkedTrackColor = WolPalette.Blue,
                    uncheckedThumbColor = WolPalette.Text2,
                    uncheckedTrackColor = WolPalette.Surface3,
                    uncheckedBorderColor = WolPalette.Line2,
                ),
            )
            Spacer(Modifier.width(12.dp))
            Text(title, style = MaterialTheme.typography.titleSmall)
        }
        // Semaine, puis week-end : sept jours tiennent mal sur une seule ligne d'un téléphone.
        for (row in listOf(WEEKDAYS, WEEKEND)) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                row.forEach { day ->
                    DayToggle(dayShort(day), dayLong(day), selected = day in state.days, enabled = enabled) { checked ->
                        if (checked) state.days.add(day) else state.days.remove(day)
                    }
                }
            }
        }
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(stringResource(R.string.hub_schedule_at), style = MaterialTheme.typography.bodyMedium, color = WolPalette.Text2)
            Spacer(Modifier.weight(1f))
            NumberMenu(state.hour, (0..23).toList(), enabled, stringResource(R.string.hub_schedule_hours)) { state.hour = it }
            Text(":", style = MonoStyle, color = WolPalette.Text2)
            NumberMenu(state.minute, (MINUTES + state.minute).distinct().sorted(), enabled, stringResource(R.string.hub_schedule_minutes)) {
                state.minute = it
            }
        }
        original?.takeIf { it.on && it.next > 0 }?.let {
            Text(stringResource(R.string.hub_schedule_next, hubTime(it.next, timezone)), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
        }
        note?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2) }
    }
}

/** Jour de la semaine à cocher. */
@Composable
private fun DayToggle(label: String, description: String, selected: Boolean, enabled: Boolean, onChange: (Boolean) -> Unit) {
    val shape = RoundedCornerShape(8.dp)
    Box(
        Modifier
            .size(width = 44.dp, height = 36.dp)
            .clip(shape)
            .background(if (selected) WolPalette.BlueSoft else WolPalette.Surface2)
            .border(1.dp, if (selected) WolPalette.Blue else WolPalette.Line2, shape)
            .toggleable(value = selected, enabled = enabled, role = Role.Checkbox, onValueChange = onChange)
            // Nom complet du jour pour les lecteurs d'écran.
            .semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        Text(
            label,
            style = MaterialTheme.typography.labelMedium,
            color = when {
                !enabled -> WolPalette.Text3
                selected -> WolPalette.Text
                else -> WolPalette.Text2
            },
        )
    }
}

/** Nombre choisi dans une liste (heures, minutes), affiché sur deux chiffres. */
@Composable
private fun NumberMenu(value: Int, choices: List<Int>, enabled: Boolean, description: String, onChange: (Int) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        TextButton(onClick = { expanded = true }, enabled = enabled, modifier = Modifier.padding(horizontal = 2.dp)) {
            Text(two(value), style = MonoStyle, color = if (enabled) WolPalette.Blue else WolPalette.Text3)
            Icon(WolIcons.ChevronDown, contentDescription = description, modifier = Modifier.size(16.dp))
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            choices.forEach { n ->
                DropdownMenuItem(text = { Text(two(n), style = MonoStyle) }, onClick = {
                    expanded = false
                    onChange(n)
                })
            }
        }
    }
}

/** Fuseau horaire des programmations du hub. */
@Composable
internal fun TimezoneDialog(current: String, onChoose: (String) -> Unit, onDismiss: () -> Unit) {
    val options = if (current in HubViewModel.TIMEZONES || current.isEmpty()) HubViewModel.TIMEZONES else listOf(current) + HubViewModel.TIMEZONES
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.hub_timezone)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stringResource(R.string.hub_timezone_help), style = MaterialTheme.typography.bodyMedium)
                options.forEach { zone ->
                    RoleChoice(zone == current, zone) { onChoose(zone) }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}
