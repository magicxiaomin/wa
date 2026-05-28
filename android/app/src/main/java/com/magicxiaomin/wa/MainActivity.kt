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
    private data class ContactItem(val jid: String, val name: String)
    private data class GroupItem(val jid: String, val name: String, val participantCount: Int)

    private var service: IBridgeService? = null
    private val selectedContactJids = linkedSetOf<String>()
    private var selectedGroupJid: String = ""
    private val pendingSends = mutableMapOf<String, PendingSend>()

    private lateinit var status: TextView
    private lateinit var qrImage: ImageView
    private lateinit var qrPayload: TextView
    private lateinit var contacts: ListView
    private lateinit var groups: ListView
    private lateinit var conversation: TextView
    private lateinit var messageInput: EditText
    private lateinit var connectButton: Button
    private lateinit var contactsButton: Button
    private lateinit var groupsButton: Button
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
            choiceMode = ListView.CHOICE_MODE_MULTIPLE
            isVerticalScrollBarEnabled = true
            scrollBarStyle = ListView.SCROLLBARS_INSIDE_INSET
        }
        groups = ListView(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                (56 * resources.displayMetrics.density * 4).toInt()
            )
            choiceMode = ListView.CHOICE_MODE_SINGLE
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
        groupsButton = Button(this).apply {
            text = "Get Groups"
            setOnClickListener { loadGroups() }
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
            addView(groupsButton)
            addView(groups)
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
            val items = mutableListOf<ContactItem>()
            val seenJids = mutableSetOf<String>()
            val array = if (json.trimStart().startsWith("[")) JSONArray(json) else JSONArray()
            for (i in 0 until array.length()) {
                val item = array.getJSONObject(i)
                val jid = item.optString("jid")
                val name = item.optString("name", jid)
                if (jid.isNotBlank() && seenJids.add(jid)) {
                    items += ContactItem(jid, name)
                }
            }
            mainHandler.post {
                selectedContactJids.clear()
                contacts.adapter = ArrayAdapter(
                    this,
                    android.R.layout.simple_list_item_multiple_choice,
                    items.map { "${it.name}\n${it.jid}" }
                )
                contacts.setOnItemClickListener { _, _, position, _ ->
                    val jid = items[position].jid
                    if (selectedContactJids.contains(jid)) {
                        selectedContactJids.remove(jid)
                    } else if (selectedContactJids.size >= MAX_MULTI_CONTACTS) {
                        contacts.setItemChecked(position, false)
                        status.text = "最多选择 $MAX_MULTI_CONTACTS 个联系人"
                        return@setOnItemClickListener
                    } else {
                        selectedContactJids.add(jid)
                        selectedGroupJid = ""
                        groups.clearChoices()
                    }
                    status.text = "Selected contacts: ${selectedContactJids.size}"
                }
                status.text = "Contacts: ${items.size}"
                updateSafetyControls()
            }
        }
    }

    private fun loadGroups() {
        if (!groupsButton.isEnabled) {
            return
        }
        runBridgeCall {
            val json = service?.groups ?: "[]"
            val items = mutableListOf<GroupItem>()
            val array = if (json.trimStart().startsWith("[")) JSONArray(json) else JSONArray()
            for (i in 0 until array.length()) {
                val item = array.getJSONObject(i)
                val jid = item.optString("jid")
                val name = item.optString("name", jid)
                val count = item.optInt("participant_count", 0)
                if (jid.isNotBlank()) {
                    items += GroupItem(jid, name, count)
                }
            }
            mainHandler.post {
                selectedGroupJid = ""
                groups.adapter = ArrayAdapter(
                    this,
                    android.R.layout.simple_list_item_single_choice,
                    items.map { "${it.name} (${it.participantCount})\n${it.jid}" }
                )
                groups.setOnItemClickListener { _, _, position, _ ->
                    selectedGroupJid = items[position].jid
                    selectedContactJids.clear()
                    contacts.clearChoices()
                    contacts.invalidateViews()
                    status.text = "Selected group: ${items[position].name}"
                }
                status.text = "Groups: ${items.size}"
                updateSafetyControls()
            }
        }
    }

    private fun sendMessage() {
        if (!sendButton.isEnabled) {
            return
        }
        val text = messageInput.text.toString()
        if (text.isBlank()) {
            status.text = "Enter text first"
            return
        }
        if (selectedGroupJid.isNotBlank()) {
            sendGroupMessage(text)
            return
        }
        if (selectedContactJids.isEmpty()) {
            status.text = "Select 1-$MAX_MULTI_CONTACTS contacts or one group first"
            return
        }
        if (selectedContactJids.size > MAX_MULTI_CONTACTS) {
            status.text = "最多选择 $MAX_MULTI_CONTACTS 个联系人"
            return
        }
        sendMultiContactMessage(text, selectedContactJids.toList())
    }

    private fun sendGroupMessage(text: String) {
        val clientMsgId = "android-${UUID.randomUUID()}"
        pendingSends[clientMsgId] = PendingSend(selectedGroupJid, text, System.currentTimeMillis())
        appendConversation("OUT group pending[${shortClientMsgId(clientMsgId)}] $selectedGroupJid: $text")
        val target = selectedGroupJid
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

    private fun sendMultiContactMessage(text: String, targets: List<String>) {
        val baseClientMsgId = "android-${UUID.randomUUID()}"
        targets.forEachIndexed { index, target ->
            val childClientMsgId = "$baseClientMsgId-${index + 1}"
            pendingSends[childClientMsgId] = PendingSend(target, text, System.currentTimeMillis())
            mainHandler.postDelayed({
                if (pendingSends.remove(childClientMsgId) != null) {
                    appendConversation("FAILED ${shortClientMsgId(childClientMsgId)} local_timeout: no terminal send event after ${UI_SEND_TIMEOUT_MS / 1000}s")
                    updateSafetyControls()
                }
            }, UI_SEND_TIMEOUT_MS)
        }
        appendConversation("OUT multi pending ${targets.size}: $text")
        val payload = JSONArray()
        targets.forEach { payload.put(it) }
        Thread {
            val result = runCatching { service?.sendTextMulti(payload.toString(), text, baseClientMsgId) ?: "[]" }
            mainHandler.post {
                result.onSuccess { json ->
                    appendMultiSendResults(json)
                }.onFailure { err ->
                    targets.forEachIndexed { index, _ ->
                        markSendFailed("$baseClientMsgId-${index + 1}", "bridge_call_failed", err.message ?: "unknown")
                    }
                }
                updateSafetyControls()
            }
        }.start()
        messageInput.setText("")
        updateSafetyControls()
    }

    private fun appendMultiSendResults(resultsJson: String) {
        val array = if (resultsJson.trimStart().startsWith("[")) JSONArray(resultsJson) else JSONArray()
        for (i in 0 until array.length()) {
            val item = array.getJSONObject(i)
            val clientMsgId = item.optString("clientMsgId")
            if (item.optBoolean("ok", false)) {
                pendingSends.remove(clientMsgId)
                appendConversation("MULTI OK ${shortClientMsgId(clientMsgId)} ${item.optString("server_msg_id")}")
            } else {
                markSendFailed(clientMsgId, item.optString("error_code", "send_failed"), item.optString("error", "unknown"))
            }
        }
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
                groupsButton.isEnabled = !riskStopped && operationWait <= 0
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
        private const val MAX_MULTI_CONTACTS = 3
    }
}
