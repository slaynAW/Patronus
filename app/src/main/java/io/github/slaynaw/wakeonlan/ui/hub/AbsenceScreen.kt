package io.github.slaynaw.wakeonlan.ui.hub

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.appContainer
import io.github.slaynaw.wakeonlan.core.hub.HubState
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.ui.admin.formatDayTime
import io.github.slaynaw.wakeonlan.ui.common.BusySpinner
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Évènements non vus du hub, pour le bandeau de la vue d'ensemble. */
data class AbsenceInfo(val hubName: String, val count: Int, val more: Boolean)

/** Évènements que le hub a vus depuis le dernier passage (null : aucun, ou pas de hub). */
fun HubState.absence(): AbsenceInfo? {
    val paired = hub ?: return null
    val events = unseen
    if (events.isEmpty()) return null
    return AbsenceInfo(paired.name, events.size, snapshot?.moreEvents == true)
}

/** Nom de chaque PC de ce téléphone, par l'empreinte de son agent (pour rédiger les évènements du hub). */
fun localNames(devices: List<Device>): Map<String, String> =
    devices.mapNotNull { d -> d.agent?.pin?.takeIf { it.isNotEmpty() }?.let { it to d.name } }.toMap()

/** « Pendant votre absence » (docs/HUB.md, §12) : évènements vus par le hub, puis marqués comme vus. */
@Composable
fun AbsenceScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val container = context.appContainer
    val hub by container.hub.state.collectAsStateWithLifecycle()
    val devices by remember(container) { container.allDevices.map { it.devices } }.collectAsStateWithLifecycle(emptyList())
    val selfShort by produceState<String?>(null, container) { value = withContext(Dispatchers.IO) { container.selfShortPin() } }
    val scope = rememberCoroutineScope()
    var busy by remember { mutableStateOf(false) }
    AbsenceContent(
        hub = hub,
        names = remember(devices) { localNames(devices) },
        selfShort = selfShort,
        busy = busy,
        onBack = onBack,
        onMarkSeen = { until ->
            if (!busy) {
                busy = true
                // D'autres évènements suivent : ils s'affichent ici après ceux-ci.
                val more = hub.snapshot?.moreEvents == true
                scope.launch {
                    try {
                        container.hub.markSeen(until)
                    } finally {
                        busy = false
                    }
                    if (!more) onBack()
                }
            }
        },
    )
}

/** Contenu de l'écran (sans état propre : captures d'écran). */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AbsenceContent(
    hub: HubState,
    names: Map<String, String>,
    selfShort: String?,
    busy: Boolean,
    onBack: () -> Unit,
    onMarkSeen: (until: Long) -> Unit,
) {
    val snapshot = hub.snapshot
    val events = hub.unseen.sortedByDescending { it.t }
    Scaffold(
        containerColor = WolPalette.Background,
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(stringResource(R.string.absence_title), style = MaterialTheme.typography.titleLarge)
                        hub.hub?.let { Text(it.name, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2) }
                    }
                },
                navigationIcon = {
                    IconButton(onClick = onBack) { Icon(WolIcons.ArrowBack, contentDescription = stringResource(R.string.back)) }
                },
                actions = { if (busy) BusySpinner(Modifier.padding(end = 12.dp)) },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = WolPalette.Background),
            )
        },
    ) { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (events.isEmpty()) {
                Text(stringResource(R.string.absence_none), style = MaterialTheme.typography.bodyMedium, color = WolPalette.Text2)
            } else {
                WolCard {
                    events.forEachIndexed { index, event ->
                        if (index > 0) RowDivider()
                        // Nom de ce téléphone pour un PC qu'il connaît, sinon celui gardé par le hub ; PC retiré : aucun.
                        val pc = snapshot?.pcs?.firstOrNull { it.id == event.pc }
                        val name = pc?.let { p -> names[p.agent].takeIf { p.agent.isNotEmpty() } ?: p.name }
                        Row(
                            Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 11.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Icon(hubEventIcon(event), contentDescription = null, tint = WolPalette.Text2, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(12.dp))
                            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                Text(hubEventText(event, name, selfShort), style = MaterialTheme.typography.bodyLarge)
                                Text(formatDayTime(event.t), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                            }
                        }
                    }
                }
            }
            if (snapshot?.moreEvents == true) {
                Text(stringResource(R.string.absence_more), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
            }
            Text(stringResource(R.string.absence_help), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
            WolButton(
                stringResource(R.string.absence_seen),
                onClick = { onMarkSeen(events.maxOf { it.t }) },
                modifier = Modifier.fillMaxWidth(),
                kind = ButtonKind.PRIMARY,
                icon = WolIcons.Check,
                enabled = events.isNotEmpty() && !busy,
                height = 48.dp,
            )
            Spacer(Modifier.height(24.dp))
        }
    }
}
