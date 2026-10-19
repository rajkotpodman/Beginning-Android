package com.beginningandroid

import android.content.Context
import android.util.Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.File
import java.net.*

class DiscoveryManager(private val context: Context) {
    private val tag = "DiscoveryManager"

    data class DiscoveredPC(
        val wsUrl: String,
        val pcName: String,
        val pairCode: String,
        val ip: String
    )

    suspend fun discoverPC(timeoutMs: Long = 2000): DiscoveredPC? = withContext(Dispatchers.IO) {
        // 1. Try UDP Broadcast Discovery
        val udpResult = discoverViaUDP(timeoutMs)
        if (udpResult != null) return@withContext udpResult

        // 2. Try Probing Subnet (Hotspot or Wi-Fi ARP table) on port 8080
        val subnetResult = probeSubnet(8080)
        if (subnetResult != null) return@withContext subnetResult

        return@withContext null
    }

    private fun discoverViaUDP(timeoutMs: Long): DiscoveredPC? {
        var socket: DatagramSocket? = null
        try {
            socket = DatagramSocket().apply {
                broadcast = true
                soTimeout = timeoutMs.toInt()
            }

            val pingMsg = "DISCOVER_BEGINNING_ANDROID".toByteArray()
            val broadcastAddr = InetAddress.getByName("255.255.255.255")
            val packet = DatagramPacket(pingMsg, pingMsg.size, broadcastAddr, 8082)
            socket.send(packet)

            val receiveBuf = ByteArray(1024)
            val receivePacket = DatagramPacket(receiveBuf, receiveBuf.size)
            val endTime = System.currentTimeMillis() + timeoutMs

            while (System.currentTimeMillis() < endTime) {
                try {
                    socket.receive(receivePacket)
                    val text = String(receivePacket.data, 0, receivePacket.length).trim()
                    if (text.startsWith("BEGINNING_ANDROID_PONG") || text.startsWith("BEGINNING_ANDROID_BEACON")) {
                        val parts = text.split("|")
                        if (parts.size >= 4) {
                            val wsUrl = parts[1]
                            val pcName = parts[2]
                            val pairCode = parts[3]
                            val ip = receivePacket.address.hostAddress ?: ""
                            Log.d(tag, "Found PC via UDP: $pcName ($wsUrl)")
                            return DiscoveredPC(wsUrl, pcName, pairCode, ip)
                        }
                    }
                } catch (e: SocketTimeoutException) {
                    break
                }
            }
        } catch (e: Exception) {
            Log.w(tag, "UDP discovery error: ${e.message}")
        } finally {
            socket?.close()
        }
        return null
    }

    private suspend fun probeSubnet(port: Int): DiscoveredPC? = withContext(Dispatchers.IO) {
        val candidates = mutableListOf<String>()

        // 1. Check ARP entries (devices connected to phone's hotspot)
        try {
            val arpLines = File("/proc/net/arp").readLines()
            for (line in arpLines) {
                val tokens = line.split("\\s+".toRegex())
                if (tokens.isNotEmpty() && tokens[0].matches(Regex("^\\d+\\.\\d+\\.\\d+\\.\\d+$")) && tokens[0] != "0.0.0.0") {
                    candidates.add(tokens[0])
                }
            }
        } catch (_: Exception) {}

        // 2. Add known hotspot subnet IPs
        if (!candidates.contains("10.250.119.105")) candidates.add("10.250.119.105")

        for (ip in candidates) {
            val result = checkHttpEndpoint(ip, port)
            if (result != null) return@withContext result
        }
        null
    }

    private fun checkHttpEndpoint(ip: String, port: Int): DiscoveredPC? {
        try {
            val url = URL("http://$ip:$port/health")
            val conn = url.openConnection() as HttpURLConnection
            conn.connectTimeout = 700
            conn.readTimeout = 700
            conn.requestMethod = "GET"
            if (conn.responseCode == 200) {
                val body = conn.inputStream.bufferedReader().readText()
                val json = JSONObject(body)
                val wsUrl = json.optString("ws_url", "ws://$ip:$port/ws")
                val pcName = json.optString("pc_name", "PC")
                val pairCode = json.optString("pair_code", "")
                Log.d(tag, "Found PC via HTTP probe: $pcName at $wsUrl")
                return DiscoveredPC(wsUrl, pcName, pairCode, ip)
            }
        } catch (_: Exception) {}
        return null
    }
}
