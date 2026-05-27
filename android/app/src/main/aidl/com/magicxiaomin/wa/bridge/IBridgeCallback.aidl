package com.magicxiaomin.wa.bridge;

interface IBridgeCallback {
    void onEvent(String eventType, String payloadJson);
}
