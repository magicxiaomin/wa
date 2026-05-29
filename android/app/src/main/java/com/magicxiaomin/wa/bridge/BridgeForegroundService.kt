package com.magicxiaomin.wa.bridge

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.os.RemoteCallbackList
import android.util.Log
import bridge.Bridge
import bridge.Client
import bridge.EventCallback

class BridgeForegroundService : Service() {
    private val callbacks = RemoteCallbackList<IBridgeCallback>()
    private val clientLock = Any()
    @Volatile private var client: Client? = null

    private val binder = object : IBridgeService.Stub() {
        override fun registerCallback(callback: IBridgeCallback) {
            callbacks.register(callback)
        }

        override fun unregisterCallback(callback: IBridgeCallback) {
            callbacks.unregister(callback)
        }

        override fun startBridge(deviceName: String) {
            ensureForeground()
            ensureClient(deviceName)
        }

        override fun connectBridge() {
            call("connect") { ensureClient("WA-Android").connect() }
        }

        override fun disconnectBridge() {
            call("disconnect") { client?.disconnect() }
        }

        override fun getState(): String {
            return runCatching { client?.state ?: "initializing" }.getOrElse {
                emit("error", """{"where":"getState","message":"${jsonEscape(it.message ?: "unknown")}"}""")
                "disconnected"
            }
        }

        override fun getSafetyStatus(): String {
            return callWithResult("getSafetyStatus", "{}") { client?.safetyStatus() ?: "{}" }
        }

        override fun getContacts(): String {
            return callWithResult("getContacts", "[]") { ensureClient("WA-Android").contacts }
        }

        override fun getGroups(): String {
            return callWithResult("getGroups", "[]") { ensureClient("WA-Android").groups }
        }

        override fun resolveJID(phoneOrJid: String): String {
            return callWithResult("resolveJID", "") { ensureClient("WA-Android").resolveJID(phoneOrJid) }
        }

        override fun sendText(to: String, text: String, clientMsgId: String) {
            call("sendText") { ensureClient("WA-Android").sendText(to, text, clientMsgId) }
        }

        override fun sendTextMulti(toJidsJson: String, text: String, clientMsgId: String): String {
            return callWithResult("sendTextMulti", "[]") {
                ensureClient("WA-Android").sendTextMulti(toJidsJson, text, clientMsgId)
            }
        }

        override fun startRemoteRelay(wsUrl: String, token: String) {
            call("startRemoteRelay") { ensureClient("WA-Android").startRemoteRelay(wsUrl, token) }
        }

        override fun stopRemoteRelay() {
            call("stopRemoteRelay") { client?.stopRemoteRelay() }
        }

        override fun getRemoteRelayStatus(): String {
            return callWithResult("getRemoteRelayStatus", "{}") { client?.remoteRelayStatus() ?: "{}" }
        }

        override fun exportTrace(path: String) {
            call("exportTrace") { ensureClient("WA-Android").exportTrace(path) }
        }

        override fun clearSession() {
            call("clearSession") {
                synchronized(clientLock) {
                    val activeClient = client ?: createClientLocked("WA-Android")
                    activeClient.clearSession()
                    client = null
                }
            }
        }
    }

    override fun onCreate() {
        super.onCreate()
        ensureForeground()
    }

    override fun onBind(intent: Intent?): IBinder = binder

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        ensureForeground()
        return START_STICKY
    }

    override fun onDestroy() {
        runCatching { client?.stop() }
        callbacks.kill()
        super.onDestroy()
    }

    private fun ensureClient(deviceName: String): Client {
        client?.let { return it }
        return synchronized(clientLock) {
            client ?: createClientLocked(deviceName)
        }
    }

    private fun createClientLocked(deviceName: String): Client {
        val dataDir = filesDir.resolve("wa-session").absolutePath
        val callback = object : EventCallback {
            override fun onEvent(eventType: String, payloadJSON: String) {
                emit(eventType, payloadJSON)
            }
        }
        return Bridge.newClient(callback, dataDir, deviceName).also {
            client = it
            it.start()
        }
    }

    private fun call(where: String, block: () -> Unit) {
        runCatching(block).onFailure {
            Log.e(TAG, where, it)
            emit("error", """{"where":"$where","message":"${jsonEscape(it.message ?: "unknown")}"}""")
        }
    }

    private fun callWithResult(where: String, fallback: String, block: () -> String): String {
        return runCatching(block).getOrElse {
            Log.e(TAG, where, it)
            emit("error", """{"where":"$where","message":"${jsonEscape(it.message ?: "unknown")}"}""")
            fallback
        }
    }

    private fun emit(eventType: String, payloadJson: String) {
        val count = callbacks.beginBroadcast()
        try {
            for (i in 0 until count) {
                runCatching {
                    callbacks.getBroadcastItem(i).onEvent(eventType, payloadJson)
                }.onFailure {
                    Log.w(TAG, "callback delivery failed: $eventType", it)
                }
            }
        } finally {
            callbacks.finishBroadcast()
        }
    }

    private fun ensureForeground() {
        val channelId = "wa_bridge"
        val manager = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(
                NotificationChannel(channelId, "WA Bridge", NotificationManager.IMPORTANCE_LOW)
            )
        }
        val notification = Notification.Builder(this, channelId)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentTitle("WA Bridge")
            .setContentText("Bridge process is running")
            .setOngoing(true)
            .build()
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun jsonEscape(value: String): String {
        return value.replace("\\", "\\\\").replace("\"", "\\\"")
    }

    companion object {
        private const val TAG = "BridgeService"
        private const val NOTIFICATION_ID = 1001
    }
}
