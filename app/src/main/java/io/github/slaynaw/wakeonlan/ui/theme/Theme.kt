package io.github.slaynaw.wakeonlan.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.slaynaw.wakeonlan.R

/**
 * Palette du thème sombre « topologie & panneau de détail », commune avec l'application Windows
 * (desktop/ui/app.css) : fond anthracite, surfaces grises, bleu d'accent, voyants lumineux.
 */
object WolPalette {
    val Background = Color(0xFF0C0E11)
    val Surface = Color(0xFF161A1F)
    val Surface2 = Color(0xFF1C2027)
    val Surface3 = Color(0xFF232830)
    val Soft = Color(0xFF1F242B)
    val Line = Color(0xFF262B33)
    val Line2 = Color(0xFF323844)
    val Text = Color(0xFFE7E9EC)
    val Text2 = Color(0xFF8A93A0)
    val Text3 = Color(0xFF6B7380)
    val Blue = Color(0xFF4A94FF)

    /** Carte graphique dans les tracés (le processeur est en [Blue]). */
    val Gpu = Color(0xFFC084FC)
    val BlueSoft = Color(0xFF14233B)
    val On = Color(0xFF2FD27A)
    val Off = Color(0xFFFF5F6B)
    val Busy = Color(0xFFFFB23F)
    val Unknown = Color(0xFF6B7380)
    val DangerBackground = Color(0xFF3A1A1F)
    val DangerText = Color(0xFFFF7A85)
    val SuccessBackground = Color(0xFF10291C)
    val SuccessText = Color(0xFF9AE8BD)
}

private val Colors = darkColorScheme(
    primary = WolPalette.Blue,
    onPrimary = Color.White,
    primaryContainer = WolPalette.BlueSoft,
    onPrimaryContainer = Color(0xFFCFE0FF),
    inversePrimary = Color(0xFF1F5FD6),
    secondary = WolPalette.Text2,
    onSecondary = WolPalette.Background,
    secondaryContainer = WolPalette.Surface3,
    onSecondaryContainer = WolPalette.Text,
    tertiary = WolPalette.On,
    onTertiary = WolPalette.Background,
    background = WolPalette.Background,
    onBackground = WolPalette.Text,
    surface = WolPalette.Background,
    onSurface = WolPalette.Text,
    surfaceVariant = WolPalette.Surface2,
    onSurfaceVariant = WolPalette.Text2,
    surfaceTint = WolPalette.Blue,
    inverseSurface = WolPalette.Text,
    inverseOnSurface = Color(0xFF111317),
    error = WolPalette.Off,
    onError = Color(0xFF1A0A0C),
    errorContainer = WolPalette.DangerBackground,
    onErrorContainer = WolPalette.DangerText,
    outline = WolPalette.Line2,
    outlineVariant = WolPalette.Line,
    scrim = Color.Black,
    surfaceBright = WolPalette.Surface3,
    surfaceDim = WolPalette.Background,
    surfaceContainerLowest = WolPalette.Background,
    surfaceContainerLow = WolPalette.Surface,
    surfaceContainer = WolPalette.Surface,
    surfaceContainerHigh = WolPalette.Surface2,
    surfaceContainerHighest = WolPalette.Surface3,
)

/** Inter (texte) et JetBrains Mono (adresses), sous licence SIL OFL (assets/licences). */
val Inter = FontFamily(
    Font(R.font.inter_regular, FontWeight.Normal),
    Font(R.font.inter_medium, FontWeight.Medium),
    Font(R.font.inter_semibold, FontWeight.SemiBold),
    Font(R.font.inter_bold, FontWeight.Bold),
)

val Mono = FontFamily(Font(R.font.jetbrains_mono_regular, FontWeight.Normal))

/** Style des adresses IP / MAC. */
val MonoStyle = TextStyle(fontFamily = Mono, fontSize = 13.sp, letterSpacing = (-0.1).sp)

private val WolTypography = Typography().let { t ->
    fun TextStyle.inter(weight: FontWeight? = null) = copy(fontFamily = Inter, fontWeight = weight ?: fontWeight)
    Typography(
        displayLarge = t.displayLarge.inter(),
        displayMedium = t.displayMedium.inter(),
        displaySmall = t.displaySmall.inter(),
        headlineLarge = t.headlineLarge.inter(FontWeight.Bold),
        headlineMedium = t.headlineMedium.inter(FontWeight.Bold),
        headlineSmall = t.headlineSmall.inter(FontWeight.Bold),
        titleLarge = t.titleLarge.inter(FontWeight.Bold),
        titleMedium = t.titleMedium.inter(FontWeight.SemiBold),
        titleSmall = t.titleSmall.inter(FontWeight.SemiBold),
        bodyLarge = t.bodyLarge.inter(),
        bodyMedium = t.bodyMedium.inter(),
        bodySmall = t.bodySmall.inter(),
        labelLarge = t.labelLarge.inter(FontWeight.SemiBold),
        labelMedium = t.labelMedium.inter(FontWeight.Medium),
        labelSmall = t.labelSmall.inter(FontWeight.Medium),
    )
}

private val WolShapes = Shapes(
    extraSmall = RoundedCornerShape(6.dp),
    small = RoundedCornerShape(8.dp),
    medium = RoundedCornerShape(12.dp),
    large = RoundedCornerShape(14.dp),
    extraLarge = RoundedCornerShape(18.dp),
)

/** Couleurs des voyants d'état. */
@Immutable
data class StatusColors(
    val online: Color,
    val offline: Color,
    val transition: Color,
    val unknown: Color,
)

private val Status = StatusColors(
    online = WolPalette.On,
    offline = WolPalette.Off,
    transition = WolPalette.Busy,
    unknown = WolPalette.Unknown,
)

val LocalStatusColors = staticCompositionLocalOf { Status }

/** Thème de l'application : toujours sombre, identique à l'application Windows. */
@Composable
fun WolTheme(content: @Composable () -> Unit) {
    CompositionLocalProvider(LocalStatusColors provides Status) {
        MaterialTheme(colorScheme = Colors, typography = WolTypography, shapes = WolShapes, content = content)
    }
}
