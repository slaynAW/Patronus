package io.github.slaynaw.wakeonlan.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

private val LightColors = lightColorScheme(
    primary = Color(0xFF1B5AB5),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFD7E3FF),
    onPrimaryContainer = Color(0xFF001B3F),
    secondary = Color(0xFF555F71),
    tertiary = Color(0xFF006B5E),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFFABC7FF),
    onPrimary = Color(0xFF002F65),
    primaryContainer = Color(0xFF00458E),
    onPrimaryContainer = Color(0xFFD7E3FF),
    secondary = Color(0xFFBDC7DC),
    tertiary = Color(0xFF5DDBC6),
)

/** Couleurs des voyants d'état, identiques quel que soit le thème dynamique (lisibilité). */
@Immutable
data class StatusColors(
    val online: Color,
    val offline: Color,
    val transition: Color,
    val unknown: Color,
)

private val LightStatus = StatusColors(
    online = Color(0xFF1E8E3E),
    offline = Color(0xFFC5221F),
    transition = Color(0xFFE37400),
    unknown = Color(0xFF80868B),
)

private val DarkStatus = StatusColors(
    online = Color(0xFF81C995),
    offline = Color(0xFFF28B82),
    transition = Color(0xFFFDD663),
    unknown = Color(0xFF9AA0A6),
)

val LocalStatusColors = staticCompositionLocalOf { LightStatus }

@Composable
fun WolTheme(darkTheme: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val context = LocalContext.current
    // Couleurs « Material You » tirées du fond d'écran (Android 12+), sinon palette bleue.
    val colors = when {
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        darkTheme -> DarkColors
        else -> LightColors
    }
    CompositionLocalProvider(
        LocalStatusColors provides if (darkTheme) DarkStatus else LightStatus,
    ) {
        MaterialTheme(colorScheme = colors, content = content)
    }
}
