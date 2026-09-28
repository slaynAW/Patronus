package io.github.slaynaw.wakeonlan.ui.overview

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.slaynaw.wakeonlan.R
import io.github.slaynaw.wakeonlan.core.model.Device
import io.github.slaynaw.wakeonlan.core.status.PowerState
import io.github.slaynaw.wakeonlan.network.LanTransport
import io.github.slaynaw.wakeonlan.ui.common.BusySpinner
import io.github.slaynaw.wakeonlan.ui.common.ButtonKind
import io.github.slaynaw.wakeonlan.ui.common.LatencyTrace
import io.github.slaynaw.wakeonlan.ui.common.RoundIconButton
import io.github.slaynaw.wakeonlan.ui.common.RowDivider
import io.github.slaynaw.wakeonlan.ui.common.StatusDot
import io.github.slaynaw.wakeonlan.ui.common.WarningBanner
import io.github.slaynaw.wakeonlan.ui.common.WolButton
import io.github.slaynaw.wakeonlan.ui.common.WolCard
import io.github.slaynaw.wakeonlan.ui.common.WolIcons
import io.github.slaynaw.wakeonlan.ui.common.rememberNow
import io.github.slaynaw.wakeonlan.ui.common.statusText
import io.github.slaynaw.wakeonlan.ui.devices.DeviceItem
import io.github.slaynaw.wakeonlan.ui.devices.DevicesUiState
import io.github.slaynaw.wakeonlan.ui.theme.LocalStatusColors
import io.github.slaynaw.wakeonlan.ui.theme.WolPalette

/** Onglet « Vue d'ensemble » : synthèse en anneau et liste compacte des PC. */
@Composable
fun OverviewTab(
    state: DevicesUiState,
    contentPadding: PaddingValues,
    onOpenDevice: (String) -> Unit,
    onAddDevice: () -> Unit,
    onWake: (Device) -> Unit,
    onShutdown: (Device) -> Unit,
    onRefresh: () -> Unit,
    onRequestPermission: () -> Unit,
) {
    val now = rememberNow()
    LazyColumn(
        contentPadding = PaddingValues(
            start = 16.dp,
            end = 16.dp,
            top = contentPadding.calculateTopPadding() + 12.dp,
            bottom = contentPadding.calculateBottomPadding() + 20.dp,
        ),
        verticalArrangement = Arrangement.spacedBy(14.dp),
        modifier = Modifier.fillMaxSize(),
    ) {
        item(key = "header") {
            ScreenHeader(stringResource(R.string.tab_overview)) {
                RoundIconButton(WolIcons.Plus, stringResource(R.string.action_add_device), onAddDevice)
                RoundIconButton(WolIcons.Refresh, stringResource(R.string.action_refresh), onRefresh)
            }
        }
        item(key = "network") { NetworkStatus(state, onRequestPermission) }
        if (state.loaded && state.items.isEmpty()) {
            item(key = "empty") { EmptyState(onAddDevice) }
        } else if (state.items.isNotEmpty()) {
            item(key = "summary") { SummaryCard(state.items) }
            item(key = "list") {
                WolCard {
                    state.items.forEachIndexed { index, item ->
                        if (index > 0) RowDivider()
                        DeviceRow(
                            item = item,
                            now = now,
                            onOpen = { onOpenDevice(item.device.id) },
                            onWake = { onWake(item.device) },
                            onShutdown = { onShutdown(item.device) },
                        )
                    }
                }
            }
        }
    }
}

/** Titre d'onglet, avec des boutons ronds à droite. */
@Composable
fun ScreenHeader(title: String, subtitle: String? = null, actions: @Composable () -> Unit = {}) {
    Row(Modifier.fillMaxWidth().padding(top = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.headlineSmall, fontSize = 26.sp)
            if (subtitle != null) Text(subtitle, style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) { actions() }
    }
}

/** Réseau du téléphone : pastille discrète, ou bandeau d'alerte s'il manque quelque chose. */
@Composable
fun NetworkStatus(state: DevicesUiState, onRequestPermission: () -> Unit) {
    when {
        !state.localNetworkGranted -> WarningBanner(
            title = stringResource(R.string.banner_permission_title),
            text = stringResource(R.string.banner_permission_text),
            action = stringResource(R.string.banner_permission_action),
            onAction = onRequestPermission,
        )
        !state.lan.connected -> WarningBanner(
            title = stringResource(R.string.banner_no_lan_title),
            text = stringResource(if (state.lan.vpnActive) R.string.banner_no_lan_vpn_text else R.string.banner_no_lan_text),
        )
        else -> {
            val transport = stringResource(
                if (state.lan.transport == LanTransport.ETHERNET) R.string.network_ethernet else R.string.network_wifi,
            )
            Row(
                Modifier
                    .clip(RoundedCornerShape(8.dp))
                    .background(WolPalette.Surface2)
                    .border(1.dp, WolPalette.Line, RoundedCornerShape(8.dp))
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                StatusDot(PowerState.ONLINE, size = 7.dp)
                Spacer(Modifier.width(8.dp))
                Text(
                    listOfNotNull(transport, state.lan.primaryAddress).joinToString(" · "),
                    style = MaterialTheme.typography.labelMedium,
                    color = WolPalette.Text2,
                )
            }
        }
    }
}

@Composable
private fun SummaryCard(items: List<DeviceItem>) {
    val colors = LocalStatusColors.current
    val on = items.count { it.status.state == PowerState.ONLINE }
    val busy = items.count { it.status.state.isTransitional }
    val off = items.count { it.status.state == PowerState.OFFLINE }
    val unknown = items.size - on - busy - off
    WolCard {
        Row(Modifier.padding(18.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(132.dp), contentAlignment = Alignment.Center) {
                SummaryRing(
                    parts = listOf(on to colors.online, busy to colors.transition, off to colors.offline, unknown to colors.unknown),
                    total = items.size,
                    modifier = Modifier.fillMaxSize(),
                )
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(
                        stringResource(R.string.summary_ratio, on, items.size),
                        style = MaterialTheme.typography.headlineMedium,
                    )
                    Text(stringResource(R.string.summary_caption), style = MaterialTheme.typography.bodySmall, color = WolPalette.Text2)
                }
            }
            Spacer(Modifier.width(22.dp))
            Column(verticalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.weight(1f)) {
                LegendRow(colors.online, stringResource(R.string.summary_on), on)
                LegendRow(colors.transition, stringResource(R.string.summary_busy), busy)
                LegendRow(colors.offline, stringResource(R.string.summary_off), off)
                LegendRow(colors.unknown, stringResource(R.string.summary_unknown), unknown)
            }
        }
    }
}

@Composable
private fun LegendRow(color: Color, label: String, count: Int) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(8.dp).clip(RoundedCornerShape(4.dp)).background(color))
        Spacer(Modifier.width(10.dp))
        Text(label, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium, modifier = Modifier.weight(1f))
        Text(count.toString(), style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Bold)
    }
}

/** Anneau de synthèse : un arc par état, proportionnel au nombre de PC. */
@Composable
private fun SummaryRing(parts: List<Pair<Int, Color>>, total: Int, modifier: Modifier = Modifier) {
    Canvas(modifier) {
        val stroke = 12.dp.toPx()
        val diameter = size.minDimension - stroke
        val topLeft = Offset((size.width - diameter) / 2, (size.height - diameter) / 2)
        val arcSize = Size(diameter, diameter)
        drawArc(WolPalette.Surface3, 0f, 360f, useCenter = false, topLeft = topLeft, size = arcSize, style = Stroke(stroke))
        if (total <= 0) return@Canvas
        val visible = parts.filter { it.first > 0 }
        // Les extrémités arrondies débordent de l'arc : l'écart entre deux arcs en tient compte.
        val capDegrees = Math.toDegrees((stroke / diameter).toDouble()).toFloat()
        val gap = if (visible.size > 1) capDegrees * 2 + 6f else 0f
        var start = -90f
        for ((count, color) in visible) {
            val sweep = 360f * count / total
            drawArc(
                color = color,
                startAngle = start + gap / 2,
                sweepAngle = (sweep - gap).coerceAtLeast(0.1f),
                useCenter = false,
                topLeft = topLeft,
                size = arcSize,
                style = Stroke(stroke, cap = StrokeCap.Round),
            )
            start += sweep
        }
    }
}

@Composable
private fun DeviceRow(item: DeviceItem, now: Long, onOpen: () -> Unit, onWake: () -> Unit, onShutdown: () -> Unit) {
    val device = item.device
    val status = item.status
    Row(
        Modifier
            .fillMaxWidth()
            .clickable(onClick = onOpen)
            .padding(start = 16.dp, end = 12.dp, top = 12.dp, bottom = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        StatusDot(status.state)
        Spacer(Modifier.width(14.dp))
        Column(Modifier.weight(1f)) {
            Text(
                device.name,
                style = MaterialTheme.typography.titleSmall,
                fontSize = 15.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                statusText(status, now),
                style = MaterialTheme.typography.bodySmall,
                color = if (status.state.isTransitional) WolPalette.Busy else WolPalette.Text2,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.width(8.dp))
        if (status.state == PowerState.ONLINE && item.latency.isNotEmpty()) {
            // Tracé du milieu de la ligne jusqu'au bouton ; le nom occupe l'autre moitié.
            LatencyTrace(item.latency, Modifier.weight(1f).height(22.dp))
            Spacer(Modifier.width(10.dp))
        }
        when {
            status.state == PowerState.ONLINE && device.canShutdown -> RoundIconButton(
                icon = WolIcons.Power,
                contentDescription = stringResource(R.string.action_shutdown),
                onClick = onShutdown,
                container = WolPalette.DangerBackground,
                content = WolPalette.DangerText,
                bordered = false,
                size = 38.dp,
            )
            status.state == PowerState.OFFLINE || status.state == PowerState.UNKNOWN -> RoundIconButton(
                icon = WolIcons.Power,
                contentDescription = stringResource(R.string.action_wake),
                onClick = onWake,
                container = WolPalette.Blue,
                content = Color.White,
                bordered = false,
                size = 38.dp,
            )
            status.state.isTransitional -> Box(Modifier.size(38.dp), contentAlignment = Alignment.Center) { BusySpinner() }
            else -> Icon(WolIcons.Chevron, contentDescription = null, tint = WolPalette.Text3, modifier = Modifier.size(18.dp))
        }
    }
}

/** Aucun PC : invitation à en ajouter un. */
@Composable
fun EmptyState(onAddDevice: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(top = 40.dp, start = 16.dp, end = 16.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Box(
            Modifier
                .size(72.dp)
                .clip(RoundedCornerShape(20.dp))
                .background(Brush.linearGradient(listOf(WolPalette.Blue, Color(0xFF1F5FD6)))),
            contentAlignment = Alignment.Center,
        ) {
            Icon(WolIcons.Power, contentDescription = null, tint = Color.White, modifier = Modifier.size(34.dp))
        }
        Spacer(Modifier.height(20.dp))
        Text(stringResource(R.string.empty_title), style = MaterialTheme.typography.titleLarge)
        Spacer(Modifier.height(8.dp))
        Text(
            stringResource(R.string.empty_text),
            style = MaterialTheme.typography.bodyMedium,
            textAlign = TextAlign.Center,
            color = WolPalette.Text2,
            modifier = Modifier.widthIn(max = 420.dp),
        )
        Spacer(Modifier.height(24.dp))
        WolButton(stringResource(R.string.action_add_device), onClick = onAddDevice, kind = ButtonKind.PRIMARY, icon = WolIcons.Plus)
    }
}
