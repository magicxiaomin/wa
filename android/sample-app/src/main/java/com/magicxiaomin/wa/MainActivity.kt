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
import android.view.View
import android.view.ViewGroup
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
    private enum class Page { LOGIN, CONTACTS, GROUPS, DEBUG }
    private data class TargetItem(val jid: String, val label: String)
    private data class PendingSend(val target: String, val text: String, val startedAtMs: Long)

    private lateinit var bridgeClient: WaBridgeClient
    private var targetMode = TargetMode.CONTACTS
    private var currentPage = Page.LOGIN
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
    private lateinit var logScroll: ScrollView
    private lateinit var messageInput: EditText
    private lateinit var resolveInput: EditText
    private lateinit var selectedSummary: TextView
    private lateinit var contentContainer: LinearLayout
    private lateinit var loginTabButton: Button
    private lateinit var contactsTabButton: Button
    private lateinit var groupsTabButton: Button
    private lateinit var debugTabButton: Button
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
    private lateinit var clearSessionButton: Button

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
            text = getString(R.string.status_starting)
            maxLines = 4
        }
        qrImage = ImageView(this).apply {
            adjustViewBounds = true
            maxHeight = dp(360)
        }
        qrPayload = TextView(this).apply {
            textSize = 10f
            setTextIsSelectable(true)
            maxLines = 4
        }
        contacts = ListView(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                dp(320)
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
            hint = getString(R.string.hint_text_message)
            setSingleLine(false)
            minLines = 2
        }
        resolveInput = EditText(this).apply {
            hint = getString(R.string.hint_resolve)
            setSingleLine(true)
        }
        selectedSummary = TextView(this).apply {
            textSize = 14f
            text = getString(R.string.no_target_selected)
        }
        contentContainer = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
        }

        connectButton = Button(this).apply {
            text = getString(R.string.connect_qr)
            setOnClickListener {
                status.text = getString(R.string.connecting)
                runBridgeCall { bridgeClient.connectBridge() }
            }
        }
        stateButton = Button(this).apply {
            text = getString(R.string.get_state)
            setOnClickListener { runJsonCall("STATE") { bridgeClient.getState() } }
        }
        safetyButton = Button(this).apply {
            text = getString(R.string.get_safety)
            setOnClickListener { runJsonCall("SAFETY") { bridgeClient.getSafetyStatus() } }
        }
        identityButton = Button(this).apply {
            text = getString(R.string.get_self_identity)
            setOnClickListener { runJsonCall("SELF") { bridgeClient.getSelfIdentity() } }
        }
        contactsButton = Button(this).apply {
            text = getString(R.string.get_contacts)
            setOnClickListener { loadContacts() }
        }
        groupsButton = Button(this).apply {
            text = getString(R.string.get_groups)
            setOnClickListener { loadGroups() }
        }
        resolveButton = Button(this).apply {
            text = getString(R.string.resolve_input_jid)
            setOnClickListener {
                val value = resolveInput.text.toString().trim()
                if (value.isBlank()) {
                    status.text = getString(R.string.enter_resolve_first)
                } else {
                    runJsonCall("RESOLVE") { bridgeClient.resolveJID(value) }
                }
            }
        }
        userInfoButton = Button(this).apply {
            text = getString(R.string.get_user_info)
            setOnClickListener { runSelectedJidsJsonCall("USER_INFO") { bridgeClient.getUserInfo(it) } }
        }
        profileButton = Button(this).apply {
            text = getString(R.string.get_profile_picture)
            setOnClickListener { runSelectedJidCall("PROFILE") { bridgeClient.getProfilePictureInfo(it) } }
        }
        markReadButton = Button(this).apply {
            text = getString(R.string.mark_read_selected)
            setOnClickListener {
                runSelectedJidCall("MARK_READ") { jid ->
                    bridgeClient.markRead(jid, JSONArray(listOf("manual-message-id")).toString(), jid)
                    """{"requested":true,"chat_jid":"$jid"}"""
                }
            }
        }
        presenceButton = Button(this).apply {
            text = getString(R.string.send_presence)
            setOnClickListener {
                runBridgeCall(onSuccess = { appendConversation(getString(R.string.presence_requested_available)) }) {
                    bridgeClient.sendPresence("available")
                }
            }
        }
        subscribePresenceButton = Button(this).apply {
            text = getString(R.string.subscribe_presence)
            setOnClickListener {
                runSelectedJidCall("SUBSCRIBE_PRESENCE") { jid ->
                    bridgeClient.subscribePresence(jid)
                    """{"requested":true,"jid":"$jid"}"""
                }
            }
        }
        sendButton = Button(this).apply {
            text = getString(R.string.send)
            setOnClickListener { sendMessage() }
        }
        disconnectButton = Button(this).apply {
            text = getString(R.string.disconnect)
            setOnClickListener {
                status.text = getString(R.string.disconnecting)
                runBridgeCall { bridgeClient.disconnectBridge() }
            }
        }
        exportTraceButton = Button(this).apply {
            text = getString(R.string.export_trace)
            setOnClickListener { exportTrace() }
        }
        exportSessionDebugButton = Button(this).apply {
            text = getString(R.string.export_session_debug)
            setOnClickListener { exportSessionDebug() }
        }
        clearSessionButton = Button(this).apply {
            text = getString(R.string.clear_session_long_press)
            setOnClickListener {
                status.text = getString(R.string.clear_session_hint)
            }
            setOnLongClickListener {
                resetUiAfterSessionClear()
                runBridgeCall(onSuccess = { updateSafetyControls() }) { bridgeClient.clearSession() }
                true
            }
        }

        loginTabButton = navButton(getString(R.string.tab_login), Page.LOGIN)
        contactsTabButton = navButton(getString(R.string.tab_contacts), Page.CONTACTS)
        groupsTabButton = navButton(getString(R.string.tab_groups), Page.GROUPS)
        debugTabButton = navButton(getString(R.string.tab_debug), Page.DEBUG)

        logScroll = ScrollView(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                dp(170)
            )
            addView(conversation)
        }

        val root = LinearLayout(this@MainActivity).apply {
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
            addView(row(loginTabButton, contactsTabButton, groupsTabButton, debugTabButton))
            addView(
                ScrollView(this@MainActivity).apply {
                    layoutParams = LinearLayout.LayoutParams(
                        LinearLayout.LayoutParams.MATCH_PARENT,
                        0,
                        1f
                    )
                    addView(contentContainer)
                }
            )
            addView(TextView(this@MainActivity).apply {
                text = getString(R.string.event_log_title)
                textSize = 13f
            })
            addView(logScroll)
        }

        setContentView(root)
        switchPage(Page.LOGIN)
    }

    private fun navButton(label: String, page: Page): Button {
        return Button(this).apply {
            text = label
            setOnClickListener { switchPage(page) }
        }
    }

    private fun switchPage(page: Page) {
        currentPage = page
        contentContainer.removeAllViews()
        loginTabButton.isEnabled = page != Page.LOGIN
        contactsTabButton.isEnabled = page != Page.CONTACTS
        groupsTabButton.isEnabled = page != Page.GROUPS
        debugTabButton.isEnabled = page != Page.DEBUG
        when (page) {
            Page.LOGIN -> showLoginPage()
            Page.CONTACTS -> showContactsPage()
            Page.GROUPS -> showGroupsPage()
            Page.DEBUG -> showDebugPage()
        }
        refreshSelectedSummary()
        updateSafetyControls()
    }

    private fun showLoginPage() {
        contentContainer.addView(section(getString(R.string.section_login)))
        contentContainer.addView(row(connectButton, stateButton))
        contentContainer.addView(row(safetyButton, identityButton))
        contentContainer.addView(qrImage)
        contentContainer.addView(qrPayload)
    }

    private fun showContactsPage() {
        targetMode = TargetMode.CONTACTS
        sendButton.text = getString(R.string.send_to_selected_contacts)
        renderContactList()
        contentContainer.addView(section(getString(R.string.section_contacts)))
        contentContainer.addView(row(contactsButton))
        contentContainer.addView(selectedSummary)
        contentContainer.addView(contacts)
        contentContainer.addView(section(getString(R.string.section_inspect_presence)))
        contentContainer.addView(resolveInput)
        contentContainer.addView(row(resolveButton, userInfoButton))
        contentContainer.addView(row(profileButton, subscribePresenceButton))
        contentContainer.addView(row(markReadButton, presenceButton))
        contentContainer.addView(section(getString(R.string.section_send_text)))
        contentContainer.addView(messageInput)
        contentContainer.addView(sendButton)
    }

    private fun showGroupsPage() {
        targetMode = TargetMode.GROUPS
        sendButton.text = getString(R.string.send_to_selected_group)
        renderGroupList()
        contentContainer.addView(section(getString(R.string.section_groups)))
        contentContainer.addView(row(groupsButton))
        contentContainer.addView(selectedSummary)
        contentContainer.addView(contacts)
        contentContainer.addView(TextView(this).apply {
            text = getString(R.string.group_receive_not_implemented)
            textSize = 13f
        })
        contentContainer.addView(section(getString(R.string.section_send_text)))
        contentContainer.addView(messageInput)
        contentContainer.addView(sendButton)
    }

    private fun showDebugPage() {
        contentContainer.addView(section(getString(R.string.section_debug)))
        contentContainer.addView(row(stateButton, safetyButton))
        contentContainer.addView(row(exportTraceButton, exportSessionDebugButton))
        contentContainer.addView(row(disconnectButton))
        contentContainer.addView(clearSessionButton)
        contentContainer.addView(TextView(this).apply {
            text = getString(R.string.debug_private_dir_hint)
            textSize = 13f
        })
    }

    private fun renderContactList() {
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
            status.text = getString(R.string.contacts_selected_fmt, selectedContactJids.size)
            refreshSelectedSummary()
            updateSafetyControls()
        }
    }

    private fun renderGroupList() {
        contacts.choiceMode = ListView.CHOICE_MODE_SINGLE
        contacts.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_single_choice, groupItems.map { it.label })
        val selectedIndex = groupItems.indexOfFirst { it.jid == selectedGroupJid }
        if (selectedIndex >= 0) {
            contacts.setItemChecked(selectedIndex, true)
        }
        contacts.setOnItemClickListener { _, _, position, _ ->
            selectedGroupJid = groupItems[position].jid
            status.text = getString(R.string.selected_group_status_fmt, groupItems[position].label)
            refreshSelectedSummary()
            updateSafetyControls()
        }
    }

    private fun refreshSelectedSummary() {
        selectedSummary.text = if (targetMode == TargetMode.GROUPS) {
            if (selectedGroupJid.isBlank()) {
                getString(R.string.selected_group_none)
            } else {
                getString(R.string.selected_group_fmt, shortJid(selectedGroupJid))
            }
        } else {
            val labels = selectedContactJids.take(3).joinToString { shortJid(it) }
            val suffix = if (selectedContactJids.size > 3) " ..." else ""
            if (labels.isBlank()) {
                getString(R.string.selected_contacts_empty)
            } else {
                getString(R.string.selected_contacts_fmt, selectedContactJids.size, labels, suffix)
            }
        }
    }

    private fun row(vararg views: View): LinearLayout {
        return LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            for (view in views) {
                (view.parent as? ViewGroup)?.removeView(view)
                addView(view, LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f))
            }
        }
    }

    private fun section(title: String): TextView {
        return TextView(this).apply {
            text = title
            textSize = 16f
        }
    }

    private fun dp(value: Int): Int {
        return (value * resources.displayMetrics.density).toInt()
    }

    private fun bindBridgeService() {
        bridgeClient = WaBridgeClient(this, "WA-Android")
        bridgeClient.setEventListener { eventType, payloadJson ->
            handleEvent(eventType, payloadJson)
        }
        runCatching {
            bridgeClient.bind()
            status.text = getString(R.string.bridge_binding)
        }.onFailure {
            status.text = getString(R.string.bridge_bind_failed_fmt, it.message)
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
                appendConversation(getString(R.string.message_in_fmt, payload.optString("from_suffix"), payload.optString("text")))
            }
            "message_sent" -> {
                val payload = JSONObject(payloadJson)
                val clientMsgId = payload.optString("clientMsgId")
                pendingSends.remove(clientMsgId)
                appendConversation(getString(R.string.message_sent_fmt, shortClientMsgId(clientMsgId), payload.optString("server_msg_id")))
            }
            "message_failed" -> {
                val payload = JSONObject(payloadJson)
                val clientMsgId = payload.optString("clientMsgId")
                pendingSends.remove(clientMsgId)
                appendConversation(
                    getString(R.string.message_failed_fmt, shortClientMsgId(clientMsgId), payload.optString("error_code"), payload.optString("error"))
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
                status.text = getString(R.string.contacts_count_fmt, items.size)
                if (currentPage != Page.CONTACTS) {
                    switchPage(Page.CONTACTS)
                } else {
                    renderContactList()
                    refreshSelectedSummary()
                }
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
                status.text = getString(R.string.groups_count_fmt, items.size)
                if (currentPage != Page.GROUPS) {
                    switchPage(Page.GROUPS)
                } else {
                    renderGroupList()
                    refreshSelectedSummary()
                }
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
            status.text = getString(R.string.enter_text_first)
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
            status.text = getString(R.string.select_contacts_first)
            return
        }
        val clientMsgId = "android-${UUID.randomUUID()}"
        sendInProgress = true
        appendConversation(getString(R.string.out_multi_pending_fmt, shortClientMsgId(clientMsgId), targets.size, text))
        Thread {
            val result = runCatching {
                bridgeClient.sendTextMulti(JSONArray(targets).toString(), text, clientMsgId)
            }
            mainHandler.post {
                sendInProgress = false
                result.onSuccess { payload ->
                    appendMultiSendResult(payload)
                }.onFailure { err ->
                    appendConversation(getString(R.string.send_failed_bridge_fmt, shortClientMsgId(clientMsgId), err.message ?: getString(R.string.unknown)))
                }
                updateSafetyControls()
            }
        }.start()
        messageInput.setText("")
        updateSafetyControls()
    }

    private fun sendGroupMessage(text: String) {
        if (selectedGroupJid.isBlank()) {
            status.text = getString(R.string.select_one_group_first)
            return
        }
        val clientMsgId = "android-${UUID.randomUUID()}"
        pendingSends[clientMsgId] = PendingSend(selectedGroupJid, text, System.currentTimeMillis())
        appendConversation(getString(R.string.out_group_pending_fmt, shortClientMsgId(clientMsgId), selectedGroupJid, text))
        val target = selectedGroupJid
        scheduleLocalSendTimeout(clientMsgId)
        Thread {
            val result = runCatching { bridgeClient.sendText(target, text, clientMsgId) }
            mainHandler.post {
                result.exceptionOrNull()?.let { err ->
                    markSendFailed(clientMsgId, "bridge_call_failed", err.message ?: getString(R.string.unknown))
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
                appendConversation(getString(R.string.local_timeout_fmt, shortClientMsgId(clientMsgId), UI_SEND_TIMEOUT_MS / 1000))
                updateSafetyControls()
            }
        }, UI_SEND_TIMEOUT_MS)
    }

    private fun exportTrace() {
        val path = filesDir.resolve("wa-trace.json").absolutePath
        runBridgeCall {
            bridgeClient.exportTrace(path)
            mainHandler.post {
                appendConversation(getString(R.string.trace_exported_fmt, path))
            }
        }
    }

    private fun exportSessionDebug() {
        val path = filesDir.resolve("wa-session-debug").absolutePath
        runBridgeCall {
            bridgeClient.exportSessionDebug(path)
            mainHandler.post {
                appendConversation(getString(R.string.session_debug_exported_fmt, path))
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
        resolveInput.setText("")
        status.text = getString(R.string.session_cleared)
        refreshSelectedSummary()
    }

    private fun runBridgeCall(onSuccess: (() -> Unit)? = null, block: () -> Unit) {
        Thread {
            val result = runCatching(block)
            mainHandler.post {
                val err = result.exceptionOrNull()
                if (err != null) {
                    status.text = getString(R.string.bridge_call_failed_fmt, err.message)
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
            status.text = getString(R.string.select_at_least_one_target)
            return
        }
        runJsonCall(label) {
            block(JSONArray(targets).toString())
        }
    }

    private fun runSelectedJidCall(label: String, block: (String) -> String) {
        val jid = currentSelectedJids().firstOrNull()
        if (jid.isNullOrBlank()) {
            status.text = getString(R.string.select_one_target)
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
        logScroll.post { logScroll.fullScroll(View.FOCUS_DOWN) }
    }

    private fun appendMultiSendResult(payload: String) {
        val array = runCatching { JSONArray(payload) }.getOrElse {
            appendConversation(getString(R.string.multi_parse_failed_fmt, payload))
            return
        }
        if (array.length() == 0) {
            appendConversation(getString(R.string.multi_result_empty))
            return
        }
        for (i in 0 until array.length()) {
            val item = array.getJSONObject(i)
            val jid = item.optString("jid", getString(R.string.unknown))
            if (item.optBoolean("ok", false)) {
                appendConversation(getString(R.string.multi_ok_fmt, jid, item.optString("server_msg_id")))
            } else {
                appendConversation(getString(R.string.multi_failed_fmt, jid, item.optString("error")))
            }
        }
    }

    private fun markSendFailed(clientMsgId: String, code: String, message: String) {
        if (pendingSends.remove(clientMsgId) != null) {
            appendConversation(getString(R.string.send_failed_fmt, shortClientMsgId(clientMsgId), code, message))
        }
    }

    private fun shortClientMsgId(clientMsgId: String): String {
        if (clientMsgId.isBlank()) {
            return getString(R.string.unknown)
        }
        return clientMsgId.takeLast(8)
    }

    private fun shortJid(jid: String): String {
        if (jid.length <= 12) {
            return jid
        }
        return "...${jid.takeLast(12)}"
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
