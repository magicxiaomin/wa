package com.magicxiaomin.wa.bridge;

import com.magicxiaomin.wa.bridge.IBridgeCallback;

interface IBridgeService {
    void registerCallback(IBridgeCallback callback);
    void unregisterCallback(IBridgeCallback callback);
    void startBridge(String deviceName);
    void connectBridge();
    void disconnectBridge();
    String getState();
    String getSafetyStatus();
    String getSelfIdentity();
    String getContacts();
    String getGroups();
    String resolveJID(String phoneOrJid);
    String getUserInfo(String jidsJson);
    String getProfilePictureInfo(String jid);
    void sendText(String to, String text, String clientMsgId);
    String sendTextMulti(String toJidsJson, String text, String clientMsgId);
    void markRead(String chatJid, String messageIdsJson, String senderJid);
    void sendPresence(String state);
    void subscribePresence(String jid);
    void exportTrace(String path);
    void exportSessionDebug(String path);
    void clearSession();
}
