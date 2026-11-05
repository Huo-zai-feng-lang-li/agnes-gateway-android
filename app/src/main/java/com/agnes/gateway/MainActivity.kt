package com.agnes.gateway

import android.annotation.SuppressLint
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.ConnectivityManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.provider.Settings
import android.text.method.ScrollingMovementMethod
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

class MainActivity : AppCompatActivity() {

    private lateinit var tvStatus: TextView
    private lateinit var tvLocalUrl: TextView
    private lateinit var tvLanUrl: TextView
    private lateinit var btnCopyUrl: Button
    private lateinit var etUpstream: EditText
    private lateinit var etPort: EditText
    private lateinit var etKeys: EditText
    private lateinit var btnReloadKeys: Button
    private lateinit var btnToggle: Button
    private lateinit var btnBatteryOpt: Button
    private lateinit var tvLogs: TextView
    private lateinit var btnClearLog: Button

    private val logStringBuilder = StringBuilder()
    private var currentBaseUrl: String = ""

    // 日志滚动保留上限：条数与时间双限，避免无界增长
    private val logLines = ArrayDeque<Pair<Long, String>>()
    private val maxLogLines = 500
    private val maxLogAgeMs = 30 * 60 * 1000L
    // 用户手动上翻时不强制回到底部，仅在其已贴底时自动跟随
    private var autoScrollEnabled = true
    private val scrollTolerancePx = 48


    private val logReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            val line = intent?.getStringExtra(GatewayService.EXTRA_LOG_LINE) ?: return
            appendLog(line)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        initViews()
        loadPreferences()
        updateUIState()
        requestNotificationPermission()
        checkForUpdates()
        autoStartGatewayIfNeeded()
    }

    // 打开 App 时自动拉起反代网关：未运行且已配置 Key 时才启动，避免空号池空转
    private fun autoStartGatewayIfNeeded() {
        if (GatewayService.isRunning) return
        if (etKeys.text.toString().trim().isEmpty()) {
            appendLog("[系统] 未配置 Key，跳过自动启动")
            return
        }
        tvStatus.postDelayed({
            if (!GatewayService.isRunning && !isFinishing && !isDestroyed) {
                appendLog("[系统] 检测到已配置 Key，自动启动反代网关")
                startGatewayService()
            }
        }, 300)
    }

    // 启动时检查 GitHub Release 是否有新版本（6 小时内只检查一次，失败静默忽略）
    private fun checkForUpdates() {
        val sp = getSharedPreferences("gateway_config", Context.MODE_PRIVATE)
        val lastCheck = sp.getLong("last_update_check", 0L)
        if (System.currentTimeMillis() - lastCheck < 6 * 60 * 60 * 1000L) return
        sp.edit().putLong("last_update_check", System.currentTimeMillis()).apply()

        Thread {
            try {
                val url = URL("https://api.github.com/repos/Huo-zai-feng-lang-li/agnes-gateway-android/releases/latest")
                val conn = (url.openConnection() as HttpURLConnection).apply {
                    connectTimeout = 8000
                    readTimeout = 8000
                    setRequestProperty("Accept", "application/vnd.github+json")
                }
                if (conn.responseCode != 200) {
                    conn.disconnect()
                    return@Thread
                }
                val body = conn.inputStream.bufferedReader().use { it.readText() }
                conn.disconnect()

                val json = JSONObject(body)
                val latestTag = json.optString("tag_name").trim().removePrefix("v")
                val htmlUrl = json.optString("html_url")
                val current = packageManager.getPackageInfo(packageName, 0).versionName ?: return@Thread

                if (latestTag.isNotEmpty() && compareVersion(latestTag, current) > 0) {
                    Handler(Looper.getMainLooper()).post {
                        AlertDialog.Builder(this)
                            .setTitle("发现新版本 v$latestTag")
                            .setMessage("当前版本 v$current，是否前往下载更新？")
                            .setPositiveButton("前往下载") { _, _ ->
                                runCatching { startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(htmlUrl))) }
                            }
                            .setNegativeButton("稍后", null)
                            .show()
                    }
                }
            } catch (_: Exception) {
                // 网络不可用或解析失败：静默忽略，不打扰用户
            }
        }.start()
    }

    // 语义化版本比较：返回 >0 表示 a 比 b 新
    private fun compareVersion(a: String, b: String): Int {
        val pa = a.split(".").map { it.toIntOrNull() ?: 0 }
        val pb = b.split(".").map { it.toIntOrNull() ?: 0 }
        for (i in 0 until maxOf(pa.size, pb.size)) {
            val va = pa.getOrElse(i) { 0 }
            val vb = pb.getOrElse(i) { 0 }
            if (va != vb) return va - vb
        }
        return 0
    }

    @SuppressLint("UnspecifiedRegisterReceiverFlag")
    override fun onResume() {
        super.onResume()
        val filter = IntentFilter(GatewayService.ACTION_LOG_BROADCAST)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerReceiver(logReceiver, filter, Context.RECEIVER_NOT_EXPORTED)
        } else {
            registerReceiver(logReceiver, filter)
        }
        updateUIState()
    }

    override fun onPause() {
        super.onPause()
        try {
            unregisterReceiver(logReceiver)
        } catch (_: Exception) {
        }
        savePreferences()
    }

    private fun initViews() {
        tvStatus = findViewById(R.id.tvStatus)
        tvLocalUrl = findViewById(R.id.tvLocalUrl)
        tvLanUrl = findViewById(R.id.tvLanUrl)
        btnCopyUrl = findViewById(R.id.btnCopyUrl)
        etUpstream = findViewById(R.id.etUpstream)
        etPort = findViewById(R.id.etPort)
        etKeys = findViewById(R.id.etKeys)
        btnReloadKeys = findViewById(R.id.btnReloadKeys)
        btnToggle = findViewById(R.id.btnToggle)
        btnBatteryOpt = findViewById(R.id.btnBatteryOpt)
        tvLogs = findViewById(R.id.tvLogs)
        tvLogs.movementMethod = ScrollingMovementMethod()
        tvLogs.setOnScrollChangeListener { _, _, _, _, _ ->
            // 仅当用户停留在底部附近时保持自动跟随，手动上翻后不再拽回
            autoScrollEnabled = isNearBottom()
        }

        btnClearLog = findViewById(R.id.btnClearLog)

        btnCopyUrl.setOnClickListener {
            val url = currentBaseUrl
            if (url.isNotBlank()) {
                val clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                val clip = ClipData.newPlainText("Agnes Gateway URL", url)
                clipboard.setPrimaryClip(clip)
                Toast.makeText(this, "已复制 Base URL 到剪贴板", Toast.LENGTH_SHORT).show()
            } else {
                Toast.makeText(this, "网关未启动", Toast.LENGTH_SHORT).show()
            }
        }

        btnReloadKeys.setOnClickListener {
            reloadKeysToRunningGateway()
        }

        btnToggle.setOnClickListener {
            if (GatewayService.isRunning) {
                stopGatewayService()
            } else {
                startGatewayService()
            }
        }

        btnBatteryOpt.setOnClickListener {
            requestIgnoreBatteryOptimization()
        }

        btnClearLog.setOnClickListener {
            logStringBuilder.clear()
            logLines.clear()
            tvLogs.text = ""
            autoScrollEnabled = true
        }
    }

    private fun reloadKeysToRunningGateway() {
        savePreferences()
        val portStr = etPort.text.toString().trim()
        val port = portStr.toIntOrNull() ?: 8080
        val keys = etKeys.text.toString().trim()

        kotlin.concurrent.thread {
            try {
                val url = java.net.URL("http://127.0.0.1:$port/_gateway/keys")
                val conn = url.openConnection() as java.net.HttpURLConnection
                conn.requestMethod = "POST"
                conn.doOutput = true
                conn.connectTimeout = 3000
                conn.readTimeout = 3000
                conn.outputStream.use { os ->
                    os.write(keys.toByteArray(Charsets.UTF_8))
                }
                val code = conn.responseCode
                runOnUiThread {
                    if (code == 200) {
                        Toast.makeText(this, "号池已热更新生效！", Toast.LENGTH_SHORT).show()
                        appendLog("[系统] 运行中号池热重载成功！")
                    } else {
                        Toast.makeText(this, "更新失败: HTTP $code", Toast.LENGTH_SHORT).show()
                    }
                }
            } catch (e: Exception) {
                runOnUiThread {
                    Toast.makeText(this, "热更新失败: ${e.message}", Toast.LENGTH_SHORT).show()
                }
            }
        }
    }

    private fun startGatewayService() {
        savePreferences()
        val portStr = etPort.text.toString().trim()
        val port = portStr.toIntOrNull() ?: 8080
        val upstream = etUpstream.text.toString().trim().ifEmpty { "https://apihub.agnes-ai.cn/v1" }
        val keys = etKeys.text.toString().trim()
        val dns = getSystemDnsServers()

        if (keys.isEmpty()) {
            Toast.makeText(this, "提示: 当前未配置 Agnes Key，请先输入 Key 号池", Toast.LENGTH_LONG).show()
        }

        val intent = Intent(this, GatewayService::class.java).apply {
            action = GatewayService.ACTION_START
            putExtra(GatewayService.EXTRA_PORT, port)
            putExtra(GatewayService.EXTRA_UPSTREAM, upstream)
            putExtra(GatewayService.EXTRA_KEYS, keys)
            putExtra(GatewayService.EXTRA_DNS, dns)
        }
        ContextCompat.startForegroundService(this, intent)

        appendLog("[系统] 正在启动网关服务...")
        tvStatus.postDelayed({ updateUIState() }, 500)
    }

    private fun stopGatewayService() {
        val intent = Intent(this, GatewayService::class.java).apply {
            action = GatewayService.ACTION_STOP
        }
        startService(intent)
        appendLog("[系统] 正在停止网关服务...")
        tvStatus.postDelayed({ updateUIState() }, 500)
    }

    private fun updateUIState() {
        val running = GatewayService.isRunning
        val port = etPort.text.toString().trim().ifEmpty { "8080" }
        val localIP = NetworkUtils.getLocalIPAddress()

        if (running) {
            tvStatus.text = "● 运行中 (Running)"
            tvStatus.setTextColor(0xFF00C853.toInt()) // 绿色
            btnToggle.text = "停止反代网关"
            btnToggle.setBackgroundColor(0xFFD32F2F.toInt()) // 红色
            etPort.isEnabled = false
            etUpstream.isEnabled = false
            btnReloadKeys.visibility = android.view.View.VISIBLE

            currentBaseUrl = "http://127.0.0.1:$port/v1"
            tvLocalUrl.text = currentBaseUrl
            tvLanUrl.text = if (localIP != null) "局域网其他设备用: http://$localIP:$port/v1" else "未检测到局域网 IP"
            btnCopyUrl.isEnabled = true
        } else {
            tvStatus.text = "○ 已停止 (Stopped)"
            tvStatus.setTextColor(0xFF9E9E9E.toInt()) // 灰色
            btnToggle.text = "启动反代网关"
            btnToggle.setBackgroundColor(0xFF2979FF.toInt()) // 蓝色
            etPort.isEnabled = true
            etUpstream.isEnabled = true
            btnReloadKeys.visibility = android.view.View.GONE

            currentBaseUrl = ""
            tvLocalUrl.text = "http://127.0.0.1:$port/v1 (未启动)"
            tvLanUrl.text = if (localIP != null) "局域网其他设备用: http://$localIP:$port/v1" else ""
            btnCopyUrl.isEnabled = false
        }
    }

    private fun appendLog(msg: String) {
        runOnUiThread {
            val now = System.currentTimeMillis()
            logLines.addLast(now to msg)

            // 裁剪可能导致前缀变化，此时需整体重建一次；否则走增量追加
            if (pruneLogs(now)) {
                rebuildLogView()
            } else {
                if (logStringBuilder.isNotEmpty()) {
                    logStringBuilder.append('\n')
                    tvLogs.append("\n")
                }
                logStringBuilder.append(msg)
                tvLogs.append(msg)
            }

            if (autoScrollEnabled) {
                tvLogs.post { scrollToBottom() }
            }
        }
    }

    // 按条数与时间双限裁剪最旧日志；返回是否发生裁剪
    private fun pruneLogs(now: Long): Boolean {
        var pruned = false
        while (logLines.isNotEmpty() && now - logLines.first().first > maxLogAgeMs) {
            logLines.removeFirst()
            pruned = true
        }
        while (logLines.size > maxLogLines) {
            logLines.removeFirst()
            pruned = true
        }
        return pruned
    }

    // 全量重建日志文本（仅裁剪时触发，频率低）
    private fun rebuildLogView() {
        logStringBuilder.clear()
        logStringBuilder.append(logLines.joinToString("\n") { it.second })
        tvLogs.text = logStringBuilder.toString()
    }

    private fun isNearBottom(): Boolean {
        val layout = tvLogs.layout ?: return true
        val bottom = layout.getLineTop(tvLogs.lineCount)
        val visibleBottom = tvLogs.scrollY + tvLogs.height
        return visibleBottom >= bottom - scrollTolerancePx
    }

    private fun scrollToBottom() {
        val layout = tvLogs.layout ?: return
        val bottom = layout.getLineTop(tvLogs.lineCount)
        tvLogs.scrollTo(0, (bottom - tvLogs.height).coerceAtLeast(0))
        autoScrollEnabled = true
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 101)
        }
    }

    private fun requestIgnoreBatteryOptimization() {
        try {
            val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
            if (!pm.isIgnoringBatteryOptimizations(packageName)) {
                val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
                    data = Uri.parse("package:$packageName")
                }
                startActivity(intent)
            } else {
                Toast.makeText(this, "已开启电池优化白名单，后台保活生效中", Toast.LENGTH_SHORT).show()
            }
        } catch (_: Exception) {
            Toast.makeText(this, "请在系统设置中允许本应用在后台常驻运行", Toast.LENGTH_SHORT).show()
        }
    }

    /**
     * 读取当前活动网络的 DNS 服务器 IP（过滤回环与链路本地地址），
     * 供内核进程绕过 Android 残缺的 /etc/resolv.conf 使用。
     */
    private fun getSystemDnsServers(): String {
        return try {
            val cm = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
            val network = cm.activeNetwork ?: return ""
            val props = cm.getLinkProperties(network) ?: return ""
            props.dnsServers
                .mapNotNull { it.hostAddress }
                .filter { it.isNotBlank() && !it.startsWith("fe80") && it != "::1" && it != "127.0.0.1" }
                .joinToString(",")
        } catch (_: Exception) {
            ""
        }
    }

    private fun loadPreferences() {
        val sp = getSharedPreferences("gateway_config", Context.MODE_PRIVATE)
        val savedUpstream = sp.getString("upstream", "https://apihub.agnes-ai.cn/v1") ?: "https://apihub.agnes-ai.cn/v1"
        // 兼容历史版本遗留的旧官网地址，自动迁移为真实 API 根地址
        etUpstream.setText(if (savedUpstream == "https://agnes-ai.cn") "https://apihub.agnes-ai.cn/v1" else savedUpstream)
        etPort.setText(sp.getString("port", "8080"))
        etKeys.setText(sp.getString("keys", ""))
    }

    private fun savePreferences() {
        val sp = getSharedPreferences("gateway_config", Context.MODE_PRIVATE)
        sp.edit()
            .putString("upstream", etUpstream.text.toString().trim())
            .putString("port", etPort.text.toString().trim())
            .putString("keys", etKeys.text.toString().trim())
            .apply()
    }
}
