package com.magicxiaomin.wa.bridge;

interface IBridgeCallback {
    oneway void onEvent(String eventType, String payloadJson);
}
