package io.github.slaynaw.wakeonlan

import android.app.Application
import android.content.Context
import io.github.slaynaw.wakeonlan.diagnostics.CrashReporter

class WolApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        CrashReporter.install(this)
        container = AppContainer(this)
    }
}

val Context.appContainer: AppContainer get() = (applicationContext as WolApplication).container
