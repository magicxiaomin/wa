package com.magicxiaomin.wa

import android.Manifest
import android.app.Activity
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.os.Build
import android.os.Bundle
import android.os.Handler
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
import com.magicxiaomin.wa.sdk.WaBridgeClient
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
    private enum class TargetMode { CONTACTS, GROUPS }
    private data class TargetItem(val jid: String, val label: String)
    private data class PendingSend(val target: String, val text: String, val startedAtMs: Long)

    private lateinit var bridgeClient: WaBridgeClient
    private var targetMode = TargetMode.CONTACTS
    private val contactItems = mutableListOf<TargetItem>()
    private val groupItems = mutableListOf<TargetItem>()
    private val selectedContactJids = linkedSetOf<String>()
    private var selectedGroupJid: String = ""
    private var sendInProgress = false
    private val pendingSends = mutableMapOf<String, PendingSend>()

    private lateinit var status: TextView
    private lateinit var qrImage: ImageView
    private lateinit var qrPayload: TextView
    private lateinit var contacts: ListView
    private lateinit var conversation: TextView
    private lateinit var messageInput: EditText
    private lateinit var connectButton: Button
    private lateinit var stateButton: Button
    private lateinit var safetyButton: Button
    private lateinit var contactsButton: Button
    private lateinit var groupsButton: Button
    private lateinit var identityButton: Button
    private lateinit var resolveButton: Button
    private lateinit var userInfoButton: Button
    private lateinit var profileButton: Button
    private lateinit var markReadButton: Button
    private lateinit var presenceButton: Button
    private lateinit var subscribePresenceButton: Button
    private lateinit var sendButton: Button
    private lateinit var disconnectButton: Button
    private lateinit var exportTraceButton: Button
    private lateinit var exportSessionDebugButton: Button

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        requestNotificationsIfNeeded()
        buildUi()
        bindBridgeService()
    }

    override fun onDestroy() {
        runCatching { bridgeClient.unbind() }
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
                runBridgeCall { bridgeClient.connectBridge() }
            }
        }
        stateButton = Button(this).apply {
            text = "Get State"
            setOnClickListener { runJsonCall("STATE") { bridgeClient.getState() } }
        }
        safetyButton = Button(this).apply {
            text = "Get Safety"
            setOnClickListener { runJsonCall("SAFETY") { bridgeClient.getSafetyStatus() } }
        }
        identityButton = Button(this).apply {
            text = "Get Self Identity"
            setOnClickListener { runJsonCall("SELF") { bridgeClient.getSelfIdentity() } }
        }
        contactsButton = Button(this).apply {
            text = "Get Contacts"
            setOnClickListener { loadContacts() }
        }
        groupsButton = Button(this).apply {
            text = "Get Groups"
            setOnClickListener { loadGroups() }
        }
        resolveButton = Button(this).apply {
            text = "Resolve Input/JID"
            setOnClickListener {
                val value = messageInput.text.toString().trim()
                if (value.isBlank()) {
                    status.text = "Enter phone or JID in text box first"
                } else {
                    runJsonCall("RESOLVE") { bridgeClient.resolveJID(value) }
                }
            }
        }
        userInfoButton = Button(this).apply {
            text = "Get User Info"
            setOnClickListener { runSelectedJidsJsonCall("USER_INFO") { bridgeClient.getUserInfo(it) } }
        }
        profileButton = Button(this).apply {
            text = "Get Profile Picture"
            setOnClickListener { runSelectedJidCall("PROFILE") { bridgeClient.getProfilePictureInfo(it) } }
        }
        markReadButton = Button(this).apply {
            text = "Mark Read (selected)"
            setOnClickListener {
                runSelectedJidCall("MARK_READ") { jid ->
                    bridgeClient.markRead(jid, JSONArray(listOf("manual-message-id")).toString(), jid)
                    """{"requested":true,"chat_jid":"$jid"}"""
                }
            }
        }
        presenceButton = Button(this).apply {
            text = "Send Presence"
            setOnClickListener {
                runBridgeCall(onSuccess = { appendConversation("PRESENCE requested available") }) {
                    bridgeClient.sendPresence("available")
                }
            }
        }
        subscribePresenceButton = Button(this).apply {
            text = "Subscribe Presence"
            setOnClickListener {
                runSelectedJidCall("SUBSCRIBE_PRESENCE") { jid ->
                    bridgeClient.subscribePresence(jid)
                    """{"requested":true,"jid":"$jid"}"""
                }
            }
        }
        sendButton = Button(this).apply {
            text = "Send"
            setOnClickListener { sendMessage() }
        }
        disconnectButton = Button(this).apply {
            text = "Disconnect"
            setOnClickListener {
                status.text = "Disconnecting..."
                runBridgeCall { bridgeClient.disconnectBridge() }
            }
        }
        exportTraceButton = Button(this).apply {
            text = "Export Trace"
            setOnClickListener { exportTrace() }
        }
        exportSessionDebugButton = Button(this).apply {
            text = "Export Session Debug"
            setOnClickListener { exportSessionDebug() }
        }
        val clear = Button(this).apply {
            text = "Clear Session (long press)"
            setOnClickListener {
                status.text = "Long press Clear Session to erase login"
            }
            setOnLongClickListener {
                resetUiAfterSessionClear()
                runBridgeCall(onSuccess = { updateSafetyControls() }) { bridgeClient.clearSession() }
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
            addView(stateButton)
            addView(safetyButton)
            addView(qrImage)
            addView(qrPayload)
            addView(identityButton)
            addView(contactsButton)
            addView(groupsButton)
            addView(contacts)
            addView(resolveButton)
            addView(userInfoButton)
            addView(profileButton)
            addView(markReadButton)
            addView(presenceButton)
            addView(subscribePresenceButton)
            addView(messageInput)
            addView(sendButton)
            addView(disconnectButton)
            addView(exportTraceButton)
            addView(exportSessionDebugButton)
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
        bridgeClient = WaBridgeClient(this, "WA-Android")
        bridgeClient.setEventListener { eventType, payloadJson ->
            handleEvent(eventType, payloadJson)
        }
        runCatching {
            bridgeClient.bind()
            status.text = "Bridge service binding..."
        }.onFailure {
            status.text = "Bridge bind failed: ${it.message}"
        }
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
            val json = bridgeClient.getContacts()
            val items = mutableListOf<TargetItem>()
            val seenJids = mutableSetOf<String>()
            val array = if (json.trimStart().startsWith("[")) JSONArray(json) else JSONArray()
            for (i in 0 until array.length()) {
                val item = array.getJSONObject(i)
                val jid = item.optString("jid")
                val name = item.optString("name", jid)
                if (jid.isNotBlank() && seenJids.add(jid)) {
                    items += TargetItem(jid, "$name\n$jid")
                }
            }
            mainHandler.post {
                targetMode = TargetMode.CONTACTS
                contactItems.clear()
                contactItems.addAll(items)
                selectedContactJids.retainAll(contactItems.map { it.jid }.toSet())
                contacts.choiceMode = ListView.CHOICE_MODE_MULTIPLE
                contacts.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_multiple_choice, contactItems.map { it.label })
                for (i in contactItems.indices) {
                    contacts.setItemChecked(i, selectedContactJids.contains(contactItems[i].jid))
                }
                contacts.setOnItemClickListener { _, _, position, _ ->
                    val jid = contactItems[position].jid
                    if (contacts.isItemChecked(position)) {
                        selectedContactJids.add(jid)
                    } else {
                        selectedContactJids.remove(jid)
                    }
                    status.text = "Contacts selected: ${selectedContactJids.size}"
                    updateSafetyControls()
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
            val json = bridgeClient.getGroups()
            val items = mutableListOf<TargetItem>()
            val seenJids = mutableSetOf<String>()
            val array = if (json.trimStart().startsWith("[")) JSONArray(json) else JSONArray()
            for (i in 0 until array.length()) {
                val item = array.getJSONObject(i)
                val jid = item.optString("jid")
                val name = item.optString("name", jid)
                val count = item.optInt("participant_count", 0)
                val suffix = if (count > 0) " ($count)" else ""
                if (jid.isNotBlank() && seenJids.add(jid)) {
                    items += TargetItem(jid, "$name$suffix\n$jid")
                }
            }
            mainHandler.post {
                targetMode = TargetMode.GROUPS
                groupItems.clear()
                groupItems.addAll(items)
                if (groupItems.none { it.jid == selectedGroupJid }) {
                    selectedGroupJid = ""
                }
                contacts.choiceMode = ListView.CHOICE_MODE_SINGLE
                contacts.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_single_choice, groupItems.map { it.label })
                val selectedIndex = groupItems.indexOfFirst { it.jid == selectedGroupJid }
                if (selectedIndex >= 0) {
                    contacts.setItemChecked(selectedIndex, true)
                }
                contacts.setOnItemClickListener { _, _, position, _ ->
                    selectedGroupJid = groupItems[position].jid
                    status.text = "Selected group: ${groupItems[position].label}"
                    updateSafetyControls()
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
        if (targetMode == TargetMode.GROUPS) {
            sendGroupMessage(text)
            return
        }
        sendMultiContactMessage(text)
    }

    private fun sendMultiContactMessage(text: String) {
        val targets = selectedContactJids.toList()
        if (targets.isEmpty()) {
            status.text = "Select contacts first"
            return
        }
        val clientMsgId = "android-${UUID.randomUUID()}"
        sendInProgress = true
        appendConversation("OUT multi pending[${shortClientMsgId(clientMsgId)}] ${targets.size} contacts: $text")
        Thread {
            val result = runCatching {
                bridgeClient.sendTextMulti(JSONArray(targets).toString(), text, clientMsgId)
            }
            mainHandler.post {
                sendInProgress = false
                result.onSuccess { payload ->
                    appendMultiSendResult(payload)
                }.onFailure { err ->
                    appendConversation("FAILED ${shortClientMsgId(clientMsgId)} bridge_call_failed: ${err.message ?: "unknown"}")
                }
                updateSafetyControls()
            }
        }.start()
        messageInput.setText("")
        updateSafetyControls()
    }

    private fun sendGroupMessage(text: String) {
        if (selectedGroupJid.isBlank()) {
            status.text = "Select one group first"
            return
        }
        val clientMsgId = "android-${UUID.randomUUID()}"
        pendingSends[clientMsgId] = PendingSend(selectedGroupJid, text, System.currentTimeMillis())
        appendConversation("OUT group pending[${shortClientMsgId(clientMsgId)}] $selectedGroupJid: $text")
        val target = selectedGroupJid
        scheduleLocalSendTimeout(clientMsgId)
        Thread {
            val result = runCatching { bridgeClient.sendText(target, text, clientMsgId) }
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

    private fun scheduleLocalSendTimeout(clientMsgId: String) {
        mainHandler.postDelayed({
            if (pendingSends.remove(clientMsgId) != null) {
                appendConversation("FAILED ${shortClientMsgId(clientMsgId)} local_timeout: no terminal send event after ${UI_SEND_TIMEOUT_MS / 1000}s")
                updateSafetyControls()
            }
        }, UI_SEND_TIMEOUT_MS)
    }

    private fun exportTrace() {
        val path = filesDir.resolve("wa-trace.json").absolutePath
        runBridgeCall {
            bridgeClient.exportTrace(path)
            mainHandler.post {
                appendConversation("TRACE exported $path")
            }
        }
    }

    private fun exportSessionDebug() {
        val path = filesDir.resolve("wa-session-debug").absolutePath
        runBridgeCall {
            bridgeClient.exportSessionDebug(path)
            mainHandler.post {
                appendConversation("SESSION_DEBUG exported $path")
            }
        }
    }

    private fun resetUiAfterSessionClear() {
        selectedContactJids.clear()
        selectedGroupJid = ""
        contactItems.clear()
        groupItems.clear()
        sendInProgress = false
        pendingSends.clear()
        contacts.adapter = null
        conversation.text = ""
        qrImage.setImageDrawable(null)
        qrPayload.text = ""
        messageInput.setText("")
        status.text = "Session cleared"
    }

    private fun runBridgeCall(onSuccess: (() -> Unit)? = null, block: () -> Unit) {
        Thread {
            val result = runCatching(block)
            mainHandler.post {
                val err = result.exceptionOrNull()
                if (err != null) {
                    status.text = "Bridge call failed: ${err.message}"
                    updateSafetyControls()
                } else {
                    onSuccess?.invoke() ?: updateSafetyControls()
                }
            }
        }.start()
    }

    private fun runJsonCall(label: String, block: () -> String) {
        runBridgeCall(onSuccess = null) {
            val payload = block()
            mainHandler.post {
                appendConversation("$label $payload")
            }
        }
    }

    private fun runSelectedJidsJsonCall(label: String, block: (String) -> String) {
        val targets = currentSelectedJids()
        if (targets.isEmpty()) {
            status.text = "Select at least one target first"
            return
        }
        runJsonCall(label) {
            block(JSONArray(targets).toString())
        }
    }

    private fun runSelectedJidCall(label: String, block: (String) -> String) {
        val jid = currentSelectedJids().firstOrNull()
        if (jid.isNullOrBlank()) {
            status.text = "Select one target first"
            return
        }
        runJsonCall(label) {
            block(jid)
        }
    }

    private fun currentSelectedJids(): List<String> {
        return if (targetMode == TargetMode.GROUPS) {
            listOfNotNull(selectedGroupJid.takeIf { it.isNotBlank() })
        } else {
            selectedContactJids.toList()
        }
    }

    private fun updateSafetyControls() {
        Thread {
            val payload = runCatching { bridgeClient.getSafetyStatus() }.getOrDefault("{}")
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
                sendButton.isEnabled = !riskStopped && sendWait <= 0 && operationWait <= 0 && pendingSends.isEmpty() && !sendInProgress
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

    private fun appendMultiSendResult(payload: String) {
        val array = runCatching { JSONArray(payload) }.getOrElse {
            appendConversation("MULTI result parse_failed: $payload")
            return
        }
        if (array.length() == 0) {
            appendConversation("MULTI result empty")
            return
        }
        for (i in 0 until array.length()) {
            val item = array.getJSONObject(i)
            val jid = item.optString("jid", "unknown")
            if (item.optBoolean("ok", false)) {
                appendConversation("MULTI ok $jid ${item.optString("server_msg_id")}")
            } else {
                appendConversation("MULTI failed $jid ${item.optString("error")}")
            }
        }
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
