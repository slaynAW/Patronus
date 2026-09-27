package io.github.slaynaw.wakeonlan.network

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.content.ContextCompat

/**
 * Autorisation « réseau local » introduite par Android 17 (API 37) : sans elle, tout le trafic vers
 * le réseau local (paquet magique, sondes, agent) est bloqué silencieusement.
 */
object LocalNetworkAccess {
    const val PERMISSION = "android.permission.ACCESS_LOCAL_NETWORK"
    private const val FIRST_ENFORCING_SDK = 37

    /** Vrai si le système du téléphone impose cette autorisation. */
    val isRequired: Boolean get() = Build.VERSION.SDK_INT >= FIRST_ENFORCING_SDK

    fun isGranted(context: Context): Boolean =
        !isRequired || ContextCompat.checkSelfPermission(context, PERMISSION) == PackageManager.PERMISSION_GRANTED
}
