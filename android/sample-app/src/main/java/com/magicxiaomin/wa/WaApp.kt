package com.magicxiaomin.wa

import android.app.Application
import android.os.Build

class WaApp : Application() {
    override fun onCreate() {
        super.onCreate()
        val process = if (Build.VERSION.SDK_INT >= 28) getProcessName() else packageName
        if (process == packageName) {
            // Main-process-only initialization will live here.
        } else if (process.endsWith(":wa_bridge")) {
            // Bridge-process-only initialization will live here.
        }
    }
}
