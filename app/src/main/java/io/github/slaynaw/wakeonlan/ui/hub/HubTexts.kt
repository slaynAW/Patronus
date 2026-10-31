package io.github.slaynaw.wakeonlan.ui.hub

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.hub.HubEvent
import io.github.slaynaw.wakeonlan.core.hub.HubPcState
import io.github.slaynaw.wakeonlan.core.hub.HubProtocol
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.formatDuration
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** État d'un PC vu par le hub : texte (codes connus seulement, T38) et couleur. */
@Composable
internal fun hubPcStateText(state: HubPcState?, now: Long): Pair<String, Color> {
    val problem = state?.problem.orEmpty()
    if (problem.isNotEmpty()) return hubProblemText(problem) to WolPalette.DangerText
    val (text, color) = when (state?.state) {
        HubPcState.ONLINE -> stringResource(R.string.hub_pc_state_online) to WolPalette.On
        HubPcState.OFFLINE -> stringResource(R.string.hub_pc_state_offline) to WolPalette.DangerText
        HubPcState.AGENT_DOWN -> stringResource(R.string.hub_pc_state_agent_down) to WolPalette.Busy
        else -> return stringResource(R.string.hub_pc_state_unknown) to WolPalette.Text2
    }
    val since = state?.since ?: 0L
    return if (since > 0) "$text · ${stringResource(R.string.hub_pc_since, formatDuration(now - since * 1000))}" to color else text to color
}

/** Problème signalé par le hub sur un PC confié (§6.2). */
@Composable
private fun hubProblemText(code: String): String = stringResource(
    when (code) {
        "revoked" -> R.string.hub_pc_problem_revoked
        "pin_mismatch" -> R.string.hub_pc_problem_pin_mismatch
        "rate_limited" -> R.string.hub_pc_problem_rate_limited
        else -> R.string.hub_pc_problem_other
    },
)

/**
 * Texte d'un évènement du hub (« Pendant votre absence »), rédigé ici à partir de ses codes (T38) : [name] est le nom
 * du PC sur ce téléphone, ou à défaut celui gardé par le hub (null : PC retiré du hub) ; [selfShort] : empreinte courte
 * de ce téléphone, pour reconnaître ses propres demandes de réveil.
 */
@Composable
internal fun hubEventText(event: HubEvent, name: String?, selfShort: String?): String {
    val who = name ?: stringResource(R.string.absence_unknown_pc)
    return when (event.k) {
        "online" -> stringResource(R.string.absence_online, who)
        "offline" -> stringResource(R.string.absence_offline, who)
        "agent_down" -> stringResource(R.string.absence_agent_down, who)
        "problem" -> stringResource(R.string.absence_problem, who, hubProblemText(event.x))
        "lost" -> stringResource(
            R.string.absence_lost,
            who,
            stringResource(
                when (event.x) {
                    "bsod" -> R.string.kind_lost_bsod
                    "button" -> R.string.kind_lost_button
                    "power" -> R.string.kind_lost_power
                    "hardware" -> R.string.kind_lost_hardware
                    else -> R.string.kind_lost
                },
            ),
        )
        "disk" -> stringResource(if (event.x == "full") R.string.absence_disk_full else R.string.absence_disk_health, who)
        "measure" -> stringResource(
            R.string.absence_measure,
            who,
            when (event.x) {
                "mem" -> stringResource(R.string.measure_name_mem)
                "battery" -> stringResource(R.string.measure_name_battery)
                else -> stringResource(R.string.measure_name_other, event.x)
            },
        )
        "wake" -> when {
            selfShort != null && event.i == selfShort -> stringResource(R.string.absence_wake_self, who)
            event.i == HubProtocol.WAKE_LOCAL -> stringResource(R.string.absence_wake_nas, who)
            else -> stringResource(R.string.absence_wake_other, who, event.i)
        }
        else -> who
    }
}

/** Icône d'un évènement du hub. */
internal fun hubEventIcon(event: HubEvent): ImageVector = when (event.k) {
    "online", "offline" -> WolIcons.Power
    "lost" -> WolIcons.Bolt
    "wake" -> WolIcons.Send
    else -> WolIcons.Warning
}

/** Bandeau « Pendant votre absence » : le hub [hubName] a vu [count] évènements ([more] : d'autres suivent). */
@Composable
fun AbsenceBanner(hubName: String, count: Int, more: Boolean, onOpen: () -> Unit, modifier: Modifier = Modifier) {
    WolCard(modifier, onClick = onOpen) {
        Row(Modifier.fillMaxWidth().padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(WolIcons.Hub, contentDescription = null, tint = WolPalette.Blue, modifier = Modifier.size(20.dp))
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(stringResource(R.string.absence_title), style = MaterialTheme.typography.titleSmall)
                Text(
                    if (count == 1) {
                        stringResource(R.string.absence_text_one, hubName)
                    } else {
                        stringResource(R.string.absence_text_many, hubName, if (more) "$count+" else count.toString())
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = WolPalette.Text2,
                )
            }
            Spacer(Modifier.width(8.dp))
            Text(stringResource(R.string.absence_open), style = MaterialTheme.typography.labelLarge, color = WolPalette.Blue)
        }
    }
}
