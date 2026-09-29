package io.github.slaynaw.wakeonlan

import android.app.Application
import android.content.Context
import io.github.slaynaw.wakeonlan.diagnostics.CrashReporter
import io.github.slaynaw.wakeonlan.diagnostics.DiagnosticLog

class WolApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        DiagnosticLog.init(this)
        CrashReporter.install(this)
        container = AppContainer(this)
    }
}

val Context.appContainer: AppContainer get() = (applicationContext as WolApplication).container
