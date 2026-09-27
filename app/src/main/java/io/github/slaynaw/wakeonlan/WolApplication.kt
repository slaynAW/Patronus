package io.github.slaynaw.wakeonlan

import android.app.Application
import android.content.Context

class WolApplication : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }
}

val Context.appContainer: AppContainer get() = (applicationContext as WolApplication).container
