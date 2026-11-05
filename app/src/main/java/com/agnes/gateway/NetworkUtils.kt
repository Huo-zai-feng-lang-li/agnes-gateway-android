package com.agnes.gateway

import java.net.Inet4Address
import java.net.NetworkInterface
import java.util.Collections

object NetworkUtils {
    /**
     * 获取手机当前的局域网 IPv4 地址 (通常以 192.168. 或 10. 或 172. 开头)
     */
    fun getLocalIPAddress(): String? {
        try {
            val interfaces = Collections.list(NetworkInterface.getNetworkInterfaces())
            // 优先匹配 wlan0 / wifi 网卡
            for (intf in interfaces) {
                if (intf.name.contains("wlan") || intf.name.contains("eth") || intf.name.contains("ap")) {
                    val addrs = Collections.list(intf.inetAddresses)
                    for (addr in addrs) {
                        if (!addr.isLoopbackAddress && addr is Inet4Address) {
                            return addr.hostAddress
                        }
                    }
                }
            }
            // 兜底遍历所有非回环 IPv4
            for (intf in interfaces) {
                val addrs = Collections.list(intf.inetAddresses)
                for (addr in addrs) {
                    if (!addr.isLoopbackAddress && addr is Inet4Address) {
                        return addr.hostAddress
                    }
                }
            }
        } catch (_: Exception) {
        }
        return null
    }
}
