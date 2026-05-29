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
    String getContacts();
    String getGroups();
    String resolveJID(String phoneOrJid);
    void sendText(String to, String text, String clientMsgId);
    String sendTextMulti(String toJidsJson, String text, String clientMsgId);
    void startRemoteRelay(String wsUrl, String token);
    void stopRemoteRelay();
    String getRemoteRelayStatus();
    void exportTrace(String path);
    void clearSession();
}
