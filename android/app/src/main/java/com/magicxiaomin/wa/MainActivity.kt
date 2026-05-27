package com.magicxiaomin.wa

import android.Manifest
import android.app.Activity
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.view.WindowInsets
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.ScrollView
import android.widget.TextView
import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import com.magicxiaomin.wa.bridge.BridgeForegroundService
import com.magicxiaomin.wa.bridge.IBridgeCallback
import com.magicxiaomin.wa.bridge.IBridgeService
import org.json.JSONArray
import org.json.JSONObject
import java.util.UUID

class MainActivity : Activity() {
    private val mainHandler = Handler(Looper.getMainLooper())
    private val safetyRefreshRunnable = object : Runnable {
        override fun run() {
            updateSafetyControls()
        }
    }
    private data class PendingSend(val target: String, val text: String, val startedAtMs: Long)

    private var service: IBridgeService? = null
    private var selectedJid: String = ""
    private val pendingSends = mutableMapOf<String, PendingSend>()

    private lateinit var status: TextView
    private lateinit var qrImage: ImageView
    private lateinit var qrPayload: TextView
    private lateinit var contacts: ListView
    private lateinit var conversation: TextView
    private lateinit var messageInput: EditText
    private lateinit var connectButton: Button
    private lateinit var contactsButton: Button
    private lateinit var sendButton: Button
    private lateinit var disconnectButton: Button
    private lateinit var exportTraceButton: Button

    private val callback = object : IBridgeCallback.Stub() {
        override fun onEvent(eventType: String, payloadJson: String) {
            mainHandler.post {
                handleEvent(eventType, payloadJson)
            }
        }
    }

    private val connection = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, binder: IBinder) {
            service = IBridgeService.Stub.asInterface(binder)
            service?.registerCallback(callback)
            service?.startBridge("WA-Android")
            status.text = "Bridge service connected: ${service?.state}"
            updateSafetyControls()
        }

        override fun onServiceDisconnected(name: ComponentName) {
            service = null
            status.text = "Bridge service disconnected"
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        requestNotificationsIfNeeded()
        buildUi()
        bindBridgeService()
    }

    override fun onDestroy() {
        runCatching { service?.unregisterCallback(callback) }
        runCatching { unbindService(connection) }
        super.onDestroy()
    }

    private fun buildUi() {
        status = TextView(this).apply {
            textSize = 15f
            text = "Starting bridge service..."
        }
        qrImage = ImageView(this).apply {
            adjustViewBounds = true
            maxHeight = 720
        }
        qrPayload = TextView(this).apply {
            textSize = 10f
            setTextIsSelectable(true)
        }
        contacts = ListView(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                (64 * resources.displayMetrics.density * 5).toInt()
            )
            isVerticalScrollBarEnabled = true
            scrollBarStyle = ListView.SCROLLBARS_INSIDE_INSET
        }
        conversation = TextView(this).apply {
            textSize = 14f
            setTextIsSelectable(true)
        }
        messageInput = EditText(this).apply {
            hint = "Text message"
            setSingleLine(false)
            minLines = 2
        }

        connectButton = Button(this).apply {
            text = "Connect / QR"
            setOnClickListener {
                status.text = "Connecting..."
                runBridgeCall { service?.connectBridge() }
            }
        }
        contactsButton = Button(this).apply {
            text = "Get Contacts"
            setOnClickListener { loadContacts() }
        }
        sendButton = Button(this).apply {
            text = "Send"
            setOnClickListener { sendMessage() }
        }
        disconnectButton = Button(this).apply {
            text = "Disconnect"
            setOnClickListener {
                status.text = "Disconnecting..."
                runBridgeCall { service?.disconnectBridge() }
            }
        }
        exportTraceButton = Button(this).apply {
            text = "Export Trace"
            setOnClickListener { exportTrace() }
        }
        val clear = Button(this).apply {
            text = "Clear Session (long press)"
            setOnClickListener {
                status.text = "Long press Clear Session to erase login"
            }
            setOnLongClickListener {
                status.text = "Session cleared"
                runBridgeCall { service?.clearSession() }
                true
            }
        }

        val content = LinearLayout(this@MainActivity).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(32, 32, 32, 32)
            setOnApplyWindowInsetsListener { view, insets ->
                var left = 0
                var top = 0
                var right = 0
                var bottom = 0
                if (Build.VERSION.SDK_INT >= 30) {
                    @Suppress("NewApi")
                    val systemBars = insets.getInsets(WindowInsets.Type.systemBars())
                    left = systemBars.left
                    top = systemBars.top
                    right = systemBars.right
                    bottom = systemBars.bottom
                } else {
                    @Suppress("DEPRECATION")
                    left = insets.systemWindowInsetLeft
                    @Suppress("DEPRECATION")
                    top = insets.systemWindowInsetTop
                    @Suppress("DEPRECATION")
                    right = insets.systemWindowInsetRight
                    @Suppress("DEPRECATION")
                    bottom = insets.systemWindowInsetBottom
                }
                view.setPadding(32 + left, 32 + top, 32 + right, 32 + bottom)
                insets
            }
            addView(status)
            addView(connectButton)
            addView(qrImage)
            addView(qrPayload)
            addView(contactsButton)
            addView(contacts)
            addView(messageInput)
            addView(sendButton)
            addView(disconnectButton)
            addView(exportTraceButton)
            addView(clear)
            addView(conversation)
        }

        setContentView(
            ScrollView(this).apply {
                addView(content)
            }
        )
    }

    private fun bindBridgeService() {
        val intent = Intent(this, BridgeForegroundService::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }
        bindService(intent, connection, Context.BIND_AUTO_CREATE)
    }

    private fun handleEvent(eventType: String, payloadJson: String) {
        status.text = "$eventType\n$payloadJson"
        when (eventType) {
            "qr_generated" -> {
                val qr = JSONObject(payloadJson).optString("qr")
                if (qr.isNotBlank()) {
                    qrImage.setImageBitmap(makeQr(qr))
                    qrPayload.text = qr
                }
            }
            "connected", "session_restored" -> Unit
            "message_received" -> {
                val payload = JSONObject(payloadJson)
                appendConversation("IN ${payload.optString("from_suffix")}: ${payload.optString("text")}")
            }
            "message_sent" -> {
                val payload = JSONObject(payloadJson)
                val clientMsgId = payload.optString("clientMsgId")
                pendingSends.remove(clientMsgId)
                appendConversation("SENT ${shortClientMsgId(clientMsgId)} ${payload.optString("server_msg_id")}")
            }
            "message_failed" -> {
                val payload = JSONObject(payloadJson)
                val clientMsgId = payload.optString("clientMsgId")
                pendingSends.remove(clientMsgId)
                appendConversation(
                    "FAILED ${shortClientMsgId(clientMsgId)} ${payload.optString("error_code")}: ${payload.optString("error")}"
                )
            }
            "error" -> appendConversation("$eventType $payloadJson")
        }
        updateSafetyControls()
    }

    private fun loadContacts() {
        if (!contactsButton.isEnabled) {
            return
        }
        runBridgeCall {
            val json = service?.contacts ?: "[]"
            val items = mutableListOf<Pair<String, String>>()
            val seenJids = mutableSetOf<String>()
            val array = if (json.trimStart().startsWith("[")) JSONArray(json) else JSONArray()
            for (i in 0 until array.length()) {
                val item = array.getJSONObject(i)
                val jid = item.optString("jid")
                val name = item.optString("name", jid)
                if (jid.isNotBlank() && seenJids.add(jid)) {
                    items += jid to name
                }
            }
            mainHandler.post {
                contacts.adapter = ArrayAdapter(
                    this,
                    android.R.layout.simple_list_item_1,
                    items.map { "${it.second}\n${it.first}" }
                )
                contacts.setOnItemClickListener { _, _, position, _ ->
                    selectedJid = items[position].first
                    status.text = "Selected $selectedJid"
                }
                status.text = "Contacts: ${items.size}"
                updateSafetyControls()
            }
        }
    }

    private fun sendMessage() {
        if (!sendButton.isEnabled) {
            return
        }
        val text = messageInput.text.toString()
        if (selectedJid.isBlank() || text.isBlank()) {
            status.text = "Select a contact and enter text first"
            return
        }
        val clientMsgId = "android-${UUID.randomUUID()}"
        pendingSends[clientMsgId] = PendingSend(selectedJid, text, System.currentTimeMillis())
        appendConversation("OUT pending[${shortClientMsgId(clientMsgId)}] $selectedJid: $text")
        val target = selectedJid
        mainHandler.postDelayed({
            if (pendingSends.remove(clientMsgId) != null) {
                appendConversation("FAILED ${shortClientMsgId(clientMsgId)} local_timeout: no terminal send event after ${UI_SEND_TIMEOUT_MS / 1000}s")
                updateSafetyControls()
            }
        }, UI_SEND_TIMEOUT_MS)
        Thread {
            val result = runCatching { service?.sendText(target, text, clientMsgId) }
            mainHandler.post {
                result.exceptionOrNull()?.let { err ->
                    markSendFailed(clientMsgId, "bridge_call_failed", err.message ?: "unknown")
                }
                updateSafetyControls()
            }
        }.start()
        messageInput.setText("")
        updateSafetyControls()
    }

    private fun exportTrace() {
        val path = filesDir.resolve("wa-trace.json").absolutePath
        runBridgeCall {
            service?.exportTrace(path)
            mainHandler.post {
                appendConversation("TRACE exported $path")
            }
        }
    }

    private fun runBridgeCall(block: () -> Unit) {
        Thread {
            val result = runCatching(block)
            mainHandler.post {
                result.exceptionOrNull()?.let { err ->
                    status.text = "Bridge call failed: ${err.message}"
                }
                updateSafetyControls()
            }
        }.start()
    }

    private fun updateSafetyControls() {
        val bridge = service ?: return
        Thread {
            val payload = runCatching { bridge.safetyStatus }.getOrDefault("{}")
            val json = runCatching { JSONObject(payload) }.getOrDefault(JSONObject())
            val riskStopped = json.optBoolean("risk_stopped", false)
            val riskWait = json.optInt("risk_retry_after_seconds", 0)
            val contactsWait = json.optInt("fresh_contacts_retry_after_seconds", 0)
            val sendWait = json.optInt("fresh_send_retry_after_seconds", 0)
            val operationWait = json.optInt("operation_retry_after_seconds", 0)
            val nextWait = listOf(riskWait, contactsWait, sendWait, operationWait)
                .filter { it > 0 }
                .minOrNull() ?: 0
            mainHandler.post {
                connectButton.isEnabled = !riskStopped
                contactsButton.isEnabled = !riskStopped && contactsWait <= 0 && operationWait <= 0
                sendButton.isEnabled = !riskStopped && sendWait <= 0 && operationWait <= 0 && pendingSends.isEmpty()
                mainHandler.removeCallbacks(safetyRefreshRunnable)
                if (nextWait > 0) {
                    val delayMs = ((nextWait.coerceAtMost(60) + 1) * 1000L)
                    mainHandler.postDelayed(safetyRefreshRunnable, delayMs)
                }
            }
        }.start()
    }

    private fun appendConversation(line: String) {
        conversation.append(line + "\n")
    }

    private fun markSendFailed(clientMsgId: String, code: String, message: String) {
        if (pendingSends.remove(clientMsgId) != null) {
            appendConversation("FAILED ${shortClientMsgId(clientMsgId)} $code: $message")
        }
    }

    private fun shortClientMsgId(clientMsgId: String): String {
        if (clientMsgId.isBlank()) {
            return "unknown"
        }
        return clientMsgId.takeLast(8)
    }

    private fun makeQr(payload: String): Bitmap {
        val size = 720
        val matrix = QRCodeWriter().encode(payload, BarcodeFormat.QR_CODE, size, size)
        val bitmap = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
        for (x in 0 until size) {
            for (y in 0 until size) {
                bitmap.setPixel(x, y, if (matrix[x, y]) 0xFF000000.toInt() else 0xFFFFFFFF.toInt())
            }
        }
        return bitmap
    }

    private fun requestNotificationsIfNeeded() {
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 100)
        }
    }

    companion object {
        private const val UI_SEND_TIMEOUT_MS = 75_000L
    }
}
