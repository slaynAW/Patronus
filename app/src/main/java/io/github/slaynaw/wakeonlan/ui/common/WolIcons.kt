package io.github.slaynaw.wakeonlan.ui.common

import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.addPathNodes
import androidx.compose.ui.unit.dp

/**
 * Icônes au trait (24 × 24), identiques à celles de l'application Windows (desktop/ui/app.js).
 * Fichier généré à partir de ces dessins : les modifier des deux côtés.
 */
object WolIcons {
    val Power: ImageVector by lazy {
        icon(
        "M12 3.5v7.5",
        "M6.7 6.9a7.5 7.5 0 1 0 10.6 0",
        )
    }
    val Topology: ImageVector by lazy {
        icon(
        "M10.2 3h3.6a1.2 1.2 0 0 1 1.2 1.2v2.6a1.2 1.2 0 0 1 -1.2 1.2h-3.6a1.2 1.2 0 0 1 -1.2 -1.2v-2.6a1.2 1.2 0 0 1 1.2 -1.2z",
        "M4.2 16h3.6a1.2 1.2 0 0 1 1.2 1.2v2.6a1.2 1.2 0 0 1 -1.2 1.2h-3.6a1.2 1.2 0 0 1 -1.2 -1.2v-2.6a1.2 1.2 0 0 1 1.2 -1.2z",
        "M16.2 16h3.6a1.2 1.2 0 0 1 1.2 1.2v2.6a1.2 1.2 0 0 1 -1.2 1.2h-3.6a1.2 1.2 0 0 1 -1.2 -1.2v-2.6a1.2 1.2 0 0 1 1.2 -1.2z",
        "M12 8v4M6 16v-2.5h12V16",
        )
    }
    val Rows: ImageVector by lazy {
        icon(
        "M9 6h11M9 12h11M9 18h11",
        "M3.7 6a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0z",
        "M3.7 12a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0z",
        "M3.7 18a0.8 0.8 0 1 0 1.6 0a0.8 0.8 0 1 0 -1.6 0z",
        )
    }
    val Settings: ImageVector by lazy {
        icon(
        "M9 12a3 3 0 1 0 6 0a3 3 0 1 0 -6 0z",
        "M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z",
        )
    }
    val Plus: ImageVector by lazy {
        icon(
        "M12 5v14M5 12h14",
        )
    }
    val Refresh: ImageVector by lazy {
        icon(
        "M20.5 12a8.5 8.5 0 1 1-2.5-6l2.5 2.5",
        "M20.5 3.5v5h-5",
        )
    }
    val Restart: ImageVector by lazy {
        icon(
        "M3.5 12a8.5 8.5 0 1 0 2.5-6L3.5 8.5",
        "M3.5 3.5v5h5",
        )
    }
    val Moon: ImageVector by lazy {
        icon(
        "M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z",
        )
    }
    val Sun: ImageVector by lazy {
        icon(
        "M8 12a4 4 0 1 0 8 0a4 4 0 1 0 -8 0z",
        "M12 2.5v2M12 19.5v2M4.6 4.6 6 6M18 18l1.4 1.4M2.5 12h2M19.5 12h2M4.6 19.4 6 18M18 6l1.4-1.4",
        )
    }
    val Edit: ImageVector by lazy {
        icon(
        "M4 20h4L19 9a2.1 2.1 0 0 0-3-3L5 17z",
        "M14 8l2 2",
        )
    }
    val Shield: ImageVector by lazy {
        icon(
        "M12 3l7.5 3v5.5c0 4.5-3.2 8-7.5 9.5-4.3-1.5-7.5-5-7.5-9.5V6z",
        "M8.8 12l2.2 2.2 4.2-4.4",
        )
    }
    val Monitor: ImageVector by lazy {
        icon(
        "M5 4h14a2 2 0 0 1 2 2v8a2 2 0 0 1 -2 2h-14a2 2 0 0 1 -2 -2v-8a2 2 0 0 1 2 -2z",
        "M8.5 20h7M12 16v4",
        )
    }
    val Hub: ImageVector by lazy {
        icon(
        "M4.5 8h15a2 2 0 0 1 2 2v4a2 2 0 0 1 -2 2h-15a2 2 0 0 1 -2 -2v-4a2 2 0 0 1 2 -2z",
        "M6 12h.01M9 12h.01M12 12h.01M15 12h.01M18 12h1",
        )
    }
    val Wifi: ImageVector by lazy {
        icon(
        "M12 19.5h.01",
        "M8.6 16a4.8 4.8 0 0 1 6.8 0",
        "M5.5 12.9a9.2 9.2 0 0 1 13 0",
        "M2.5 9.7a13.4 13.4 0 0 1 19 0",
        )
    }
    val Chevron: ImageVector by lazy {
        icon(
        "M9 5l7 7-7 7",
        )
    }
    val ChevronDown: ImageVector by lazy {
        icon(
        "M6 9l6 6 6-6",
        )
    }
    val ChevronUp: ImageVector by lazy {
        icon(
        "M6 15l6-6 6 6",
        )
    }
    /** Flèche de retour (propre à Android : la fenêtre Windows n'a pas de navigation arrière). */
    val ArrowBack: ImageVector by lazy {
        icon(
        "M19 12H5",
        "M11 6l-6 6 6 6",
        )
    }
    val ArrowUp: ImageVector by lazy {
        icon(
        "M12 19V5M6 11l6-6 6 6",
        )
    }
    val ArrowDown: ImageVector by lazy {
        icon(
        "M12 5v14M6 13l6 6 6-6",
        )
    }
    val More: ImageVector by lazy {
        icon(
        "M4.3 12a1.2 1.2 0 1 0 2.4 0a1.2 1.2 0 1 0 -2.4 0z",
        "M10.8 12a1.2 1.2 0 1 0 2.4 0a1.2 1.2 0 1 0 -2.4 0z",
        "M17.3 12a1.2 1.2 0 1 0 2.4 0a1.2 1.2 0 1 0 -2.4 0z",
        )
    }
    val Delete: ImageVector by lazy {
        icon(
        "M4 7h16M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13",
        "M10 11v5M14 11v5",
        )
    }
    val Close: ImageVector by lazy {
        icon(
        "M6 6l12 12M18 6 6 18",
        )
    }
    val Warning: ImageVector by lazy {
        icon(
        "M12 3.5 2.5 20h19z",
        "M12 10v4.5M12 17.5h.01",
        )
    }
    val Check: ImageVector by lazy {
        icon(
        "M3 12a9 9 0 1 0 18 0a9 9 0 1 0 -18 0z",
        "M8.5 12.2l2.4 2.4 4.8-5",
        )
    }
    val Paste: ImageVector by lazy {
        icon(
        "M7 4.5h10a2 2 0 0 1 2 2v12.5a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2v-12.5a2 2 0 0 1 2 -2z",
        "M9 4.5V3h6v1.5M9 11h6M9 15h4",
        )
    }
    val History: ImageVector by lazy {
        icon(
        "M3.5 12a8.5 8.5 0 1 0 2.6-6.1",
        "M3.5 4.5v4h4",
        "M12 7.5V12l3 2",
        )
    }
    val Download: ImageVector by lazy {
        icon(
        "M12 4v11M7 10.5l5 5 5-5",
        "M4.5 19.5h15",
        )
    }
    val Upload: ImageVector by lazy {
        icon(
        "M12 16V5M7 9.5l5-5 5 5",
        "M4.5 19.5h15",
        )
    }
    val Info: ImageVector by lazy {
        icon(
        "M3 12a9 9 0 1 0 18 0a9 9 0 1 0 -18 0z",
        "M12 11v5.5M12 7.5h.01",
        )
    }
    val Code: ImageVector by lazy {
        icon(
        "M8.5 7 3.5 12l5 5M15.5 7l5 5-5 5",
        )
    }
    val Lock: ImageVector by lazy {
        icon(
        "M7 10.5h10a2 2 0 0 1 2 2v6a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2v-6a2 2 0 0 1 2 -2z",
        "M8 10.5V7.5a4 4 0 0 1 8 0v3",
        )
    }
    val Clock: ImageVector by lazy {
        icon(
        "M3.5 12a8.5 8.5 0 1 0 17 0a8.5 8.5 0 1 0 -17 0z",
        "M12 7.5V12l3 2",
        )
    }
    val Bolt: ImageVector by lazy {
        icon(
        "M13 2.5 4.5 13.5H12l-1 8 8.5-11H12z",
        )
    }
    val Thermometer: ImageVector by lazy {
        icon(
        "M14 14.8V4.5a2 2 0 0 0-4 0v10.3a4 4 0 1 0 4 0z",
        "M12 9.5v7.5",
        )
    }
    val Send: ImageVector by lazy {
        icon(
        "M21 3 10.5 13.5",
        "M21 3l-6.5 18-4-7.5L3 9.5z",
        )
    }

    val Share: ImageVector by lazy {
        icon(
        "M15.5 5.5a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0 -5 0z",
        "M3.5 12a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0 -5 0z",
        "M15.5 18.5a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0 -5 0z",
        "M8.2 10.8l7.6-4.1M8.2 13.2l7.6 4.1",
        )
    }
    val Users: ImageVector by lazy {
        icon(
        "M5.5 8a3.5 3.5 0 1 0 7 0a3.5 3.5 0 1 0 -7 0z",
        "M2.5 20a6.5 6.5 0 0 1 13 0",
        "M16 4.6a3.5 3.5 0 0 1 0 6.8M18.5 14.2a6.5 6.5 0 0 1 3 5.8",
        )
    }
    val Copy: ImageVector by lazy {
        icon(
        "M10.5 8.5h8a2 2 0 0 1 2 2v8a2 2 0 0 1 -2 2h-8a2 2 0 0 1 -2 -2v-8a2 2 0 0 1 2 -2z",
        "M15.5 8.5v-3a2 2 0 0 0-2-2h-8a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h3",
        )
    }
    val Cloud: ImageVector by lazy {
        icon(
        "M7 18.5h10.5a4 4 0 0 0 .6-8 6 6 0 0 0-11.6-1.4A4.8 4.8 0 0 0 7 18.5z",
        )
    }
    val Chart: ImageVector by lazy {
        icon(
        "M3.5 20.5h17",
        "M4.5 16.5l4.5-5 3.5 3 6.5-8",
        )
    }
    val Eye: ImageVector by lazy {
        icon(
        "M2.5 12s3.5-6.5 9.5-6.5 9.5 6.5 9.5 6.5-3.5 6.5-9.5 6.5S2.5 12 2.5 12z",
        "M9.2 12a2.8 2.8 0 1 0 5.6 0a2.8 2.8 0 1 0 -5.6 0z",
        )
    }
    val Scan: ImageVector by lazy {
        icon(
        "M4 8V5.5A1.5 1.5 0 0 1 5.5 4H8M16 4h2.5A1.5 1.5 0 0 1 20 5.5V8M20 16v2.5a1.5 1.5 0 0 1-1.5 1.5H16M8 20H5.5A1.5 1.5 0 0 1 4 18.5V16",
        "M4 12h16",
        )
    }

    private fun icon(vararg paths: String): ImageVector {
        val builder = ImageVector.Builder(defaultWidth = 24.dp, defaultHeight = 24.dp, viewportWidth = 24f, viewportHeight = 24f)
        for (d in paths) {
            builder.addPath(
                pathData = addPathNodes(d),
                fill = null,
                stroke = SolidColor(androidx.compose.ui.graphics.Color.Black),
                strokeLineWidth = 1.7f,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
        }
        return builder.build()
    }
}
