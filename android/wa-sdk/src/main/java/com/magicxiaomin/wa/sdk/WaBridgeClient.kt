package com.magicxiaomin.wa.sdk

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.RemoteException
import com.magicxiaomin.wa.bridge.BridgeForegroundService
import com.magicxiaomin.wa.bridge.IBridgeCallback
import com.magicxiaomin.wa.bridge.IBridgeService

class WaBridgeException(message: String, cause: Throwable? = null) : RuntimeException(message, cause)

fun interface WaBridgeEventListener {
    fun onEvent(eventType: String, payloadJson: String)
}

class WaBridgeClient(
    context: Context,
    private val deviceName: String = "WA-Android"
) {
    private val appContext = context.applicationContext
    private val mainHandler = Handler(Looper.getMainLooper())
    private var service: IBridgeService? = null
    private var listener: WaBridgeEventListener? = null
    private var bound = false

    private val callback = object : IBridgeCallback.Stub() {
        override fun onEvent(eventType: String, payloadJson: String) {
            mainHandler.post {
                listener?.onEvent(eventType, payloadJson)
            }
        }
    }

    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, binder: IBinder) {
            val bridge = IBridgeService.Stub.asInterface(binder)
            service = bridge
            runBridge("registerCallback") { bridge.registerCallback(callback) }
            runBridge("startBridge") { bridge.startBridge(deviceName) }
        }

        override fun onServiceDisconnected(name: ComponentName) {
            service = null
        }
    }

    fun setEventListener(listener: WaBridgeEventListener?) {
        this.listener = listener
    }

    fun bind() {
        if (bound) return
        val intent = Intent(appContext, BridgeForegroundService::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            appContext.startForegroundService(intent)
        } else {
            appContext.startService(intent)
        }
        bound = appContext.bindService(intent, connection, Context.BIND_AUTO_CREATE)
        if (!bound) {
            throw WaBridgeException("bindService returned false")
        }
    }

    fun unbind() {
        val bridge = service
        if (bridge != null) {
            runCatching { bridge.unregisterCallback(callback) }
        }
        if (bound) {
            runCatching { appContext.unbindService(connection) }
        }
        bound = false
        service = null
    }

    fun connectBridge() = runBridge("connectBridge") { requireService().connectBridge() }

    fun disconnectBridge() = runBridge("disconnectBridge") { requireService().disconnectBridge() }

    fun clearSession() = runBridge("clearSession") { requireService().clearSession() }

    fun getState(): String = runBridgeResult("getState") { requireService().state }

    fun getSafetyStatus(): String = runBridgeResult("getSafetyStatus") { requireService().safetyStatus }

    fun getSelfIdentity(): String = runBridgeResult("getSelfIdentity") { requireService().selfIdentity }

    fun getContacts(): String = runBridgeResult("getContacts") { requireService().contacts }

    fun getGroups(): String = runBridgeResult("getGroups") { requireService().groups }

    fun resolveJID(phoneOrJid: String): String = runBridgeResult("resolveJID") { requireService().resolveJID(phoneOrJid) }

    fun getUserInfo(jidsJson: String): String = runBridgeResult("getUserInfo") { requireService().getUserInfo(jidsJson) }

    fun getProfilePictureInfo(jid: String): String = runBridgeResult("getProfilePictureInfo") {
        requireService().getProfilePictureInfo(jid)
    }

    fun sendText(to: String, text: String, clientMsgId: String) = runBridge("sendText") {
        requireService().sendText(to, text, clientMsgId)
    }

    fun sendTextMulti(toJidsJson: String, text: String, clientMsgId: String): String = runBridgeResult("sendTextMulti") {
        requireService().sendTextMulti(toJidsJson, text, clientMsgId)
    }

    fun markRead(chatJid: String, messageIdsJson: String, senderJid: String) = runBridge("markRead") {
        requireService().markRead(chatJid, messageIdsJson, senderJid)
    }

    fun sendPresence(state: String) = runBridge("sendPresence") {
        requireService().sendPresence(state)
    }

    fun subscribePresence(jid: String) = runBridge("subscribePresence") {
        requireService().subscribePresence(jid)
    }

    fun exportTrace(path: String) = runBridge("exportTrace") {
        requireService().exportTrace(path)
    }

    fun exportSessionDebug(path: String) = runBridge("exportSessionDebug") {
        requireService().exportSessionDebug(path)
    }

    private fun requireService(): IBridgeService {
        return service ?: throw WaBridgeException("bridge service is not connected")
    }

    private fun runBridge(where: String, block: () -> Unit) {
        try {
            block()
        } catch (err: RemoteException) {
            throw WaBridgeException("$where failed across process boundary: ${err.message}", err)
        } catch (err: RuntimeException) {
            if (err is WaBridgeException) throw err
            throw WaBridgeException("$where failed: ${err.message}", err)
        }
    }

    private fun <T> runBridgeResult(where: String, block: () -> T): T {
        try {
            return block()
        } catch (err: RemoteException) {
            throw WaBridgeException("$where failed across process boundary: ${err.message}", err)
        } catch (err: RuntimeException) {
            if (err is WaBridgeException) throw err
            throw WaBridgeException("$where failed: ${err.message}", err)
        }
    }
}
