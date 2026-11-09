package com.agnes.gateway

import android.annotation.SuppressLint
import android.app.Dialog
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.ActivityInfo
import android.graphics.Typeface
import android.net.ConnectivityManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.provider.Settings
import android.text.Spannable
import android.text.SpannableStringBuilder
import android.text.Spanned
import android.text.method.ScrollingMovementMethod
import android.text.style.ForegroundColorSpan
import android.text.style.StyleSpan
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.util.ArrayDeque

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

    // 实时运维面板控件
    private lateinit var cardPoolDashboard: LinearLayout
    private lateinit var tvPoolHealth: TextView
    private lateinit var tvCurrentKey: TextView
    private lateinit var tvAvailableKeys: TextView
    private lateinit var tvCooldownKeys: TextView
    private lateinit var tvBannedKeys: TextView
    private lateinit var layoutAlertBanner: LinearLayout
    private lateinit var tvAlertBanner: TextView

    private val logStringBuilder = SpannableStringBuilder()
    private var currentBaseUrl: String = ""
    private var fullscreenLogDialog: Dialog? = null
    private var fullscreenTvLogs: TextView? = null

    // 状态轮询器
    private val statusHandler = Handler(Looper.getMainLooper())
    private var isPollingStatus = false
    private var hasNotifiedAllUnavailable = false
    private var isSyncingBanned = false

    // 日志滚动保留上限：条数与时间双限，避免无界增长
    private val logLines = ArrayDeque<Pair<Long, String>>()
    private val maxLogLines = 500
    private val maxLogAgeMs = 30 * 60 * 1000L
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

    // 语义化版本比较：返回 >0 表示 a 比 b 新（稳健过滤非数字后缀）
    private fun compareVersion(a: String, b: String): Int {
        fun normalize(s: String) = s.trim().removePrefix("v")
            .split(".")
            .map { it.takeWhile { c -> c.isDigit() }.toIntOrNull() ?: 0 }

        val pa = normalize(a)
        val pb = normalize(b)
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
        startStatusPolling()
    }

    override fun onPause() {
        super.onPause()
        stopStatusPolling()
        try {
            unregisterReceiver(logReceiver)
        } catch (_: Exception) {
        }
        savePreferences()
    }

    override fun onDestroy() {
        super.onDestroy()
        stopStatusPolling()
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

        // 实时运维面板控件
        cardPoolDashboard = findViewById(R.id.cardPoolDashboard)
        tvPoolHealth = findViewById(R.id.tvPoolHealth)
        tvCurrentKey = findViewById(R.id.tvCurrentKey)
        tvAvailableKeys = findViewById(R.id.tvAvailableKeys)
        tvCooldownKeys = findViewById(R.id.tvCooldownKeys)
        tvBannedKeys = findViewById(R.id.tvBannedKeys)
        layoutAlertBanner = findViewById(R.id.layoutAlertBanner)
        tvAlertBanner = findViewById(R.id.tvAlertBanner)

        // 阻止外层 ScrollView 抢走触摸事件，让日志框能够独立上下滑动
        tvLogs.setOnTouchListener { v, event ->
            when (event.action) {
                android.view.MotionEvent.ACTION_DOWN -> {
                    v.parent?.requestDisallowInterceptTouchEvent(true)
                    autoScrollEnabled = isNearBottom()
                }
                android.view.MotionEvent.ACTION_UP,
                android.view.MotionEvent.ACTION_CANCEL -> {
                    v.parent?.requestDisallowInterceptTouchEvent(false)
                }
            }
            false
        }
        tvLogs.setOnClickListener {
            toggleFullscreenLog()
        }
        tvLogs.setOnScrollChangeListener { _, _, _, _, _ ->
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
            fullscreenTvLogs?.text = ""
            autoScrollEnabled = true
        }
    }

    // 周期性拉取网关核心状态并刷新运维面板
    private val statusRunnable = object : Runnable {
        override fun run() {
            if (GatewayService.isRunning) {
                fetchGatewayStatus()
            }
            if (isPollingStatus) {
                statusHandler.postDelayed(this, 1500)
            }
        }
    }

    private fun startStatusPolling() {
        if (!isPollingStatus) {
            isPollingStatus = true
            statusHandler.post(statusRunnable)
        }
    }

    private fun stopStatusPolling() {
        isPollingStatus = false
        statusHandler.removeCallbacks(statusRunnable)
    }

    private fun fetchGatewayStatus(onComplete: ((JSONObject?) -> Unit)? = null) {
        val portStr = etPort.text.toString().trim()
        val port = portStr.toIntOrNull() ?: 8080
        kotlin.concurrent.thread {
            try {
                val url = URL("http://127.0.0.1:$port/_gateway/status")
                val conn = (url.openConnection() as HttpURLConnection).apply {
                    connectTimeout = 1500
                    readTimeout = 1500
                }
                if (conn.responseCode == 200) {
                    val body = conn.inputStream.bufferedReader().use { it.readText() }
                    conn.disconnect()
                    val json = JSONObject(body)
                    runOnUiThread {
                        updateDashboardUI(json)
                        onComplete?.invoke(json)
                    }
                } else {
                    conn.disconnect()
                    runOnUiThread { onComplete?.invoke(null) }
                }
            } catch (_: Exception) {
                runOnUiThread { onComplete?.invoke(null) }
            }
        }
    }

    private fun updateDashboardUI(json: JSONObject) {
        val lastUsed = json.optString("last_used_key").trim()
        val total = json.optInt("total_keys", 0)
        val available = json.optInt("available_keys", 0)
        val cooldown = json.optInt("cooldown_keys", 0)
        val banned = json.optInt("banned_keys", 0)

        tvCurrentKey.text = if (lastUsed.isNotEmpty()) "当前 Key: $lastUsed" else "当前 Key: 空闲待命"
        tvAvailableKeys.text = "可用: $available"
        tvCooldownKeys.text = "限流: $cooldown"
        tvBannedKeys.text = "封禁: $banned"

        if (total == 0 && banned > 0) {
            // 所有 Key 均被 401 封禁物理剔除，维持最高级别警报
            tvPoolHealth.text = "全部已封禁"
            tvPoolHealth.setTextColor(0xFFFF1744.toInt())
            layoutAlertBanner.visibility = View.VISIBLE
            tvAlertBanner.text = "⚠️ 严重警告：号池中全部 $banned 个 Key 均已 401 封禁失效，号池已空！"

            if (!hasNotifiedAllUnavailable) {
                hasNotifiedAllUnavailable = true
                GatewayService.triggerAllKeysUnavailableAlert(this)
                if (!isFinishing && !isDestroyed) {
                    AlertDialog.Builder(this)
                        .setTitle("⚠️ 号池告警通知")
                        .setMessage("所有配置的 Key 均已被上游 401 封禁失效并从号池移除！\n\n反代网关已无可用凭证，请补充新的有效 Key。")
                        .setPositiveButton("我知道了", null)
                        .show()
                }
            }
        } else if (total == 0) {
            tvPoolHealth.text = "号池为空"
            tvPoolHealth.setTextColor(0xFFFFA000.toInt())
            layoutAlertBanner.visibility = View.GONE
            hasNotifiedAllUnavailable = false
        } else if (available > 0) {
            tvPoolHealth.text = "号池正常"
            tvPoolHealth.setTextColor(0xFF00E676.toInt())
            layoutAlertBanner.visibility = View.GONE
            hasNotifiedAllUnavailable = false
        } else {
            // total > 0 && available == 0：全量不可用触发告警
            tvPoolHealth.text = "全部不可用"
            tvPoolHealth.setTextColor(0xFFFF1744.toInt())
            layoutAlertBanner.visibility = View.VISIBLE
            tvAlertBanner.text = "⚠️ 警告：当前所有 Key 均不可用！已全部被限流或已失效"

            if (!hasNotifiedAllUnavailable) {
                hasNotifiedAllUnavailable = true
                GatewayService.triggerAllKeysUnavailableAlert(this)
                if (!isFinishing && !isDestroyed) {
                    AlertDialog.Builder(this)
                        .setTitle("⚠️ 号池告警通知")
                        .setMessage("当前所有 Key 均处于限流冷却中或已封禁失效，反代网关已无法响应请求！\n\n请等待冷却恢复或补充新的有效 Key。")
                        .setPositiveButton("我知道了", null)
                        .show()
                }
            }
        }
    }

    // 检测到 401 封禁后，将失效 Key 从本地 etKeys 中自动移除并保存（精准黑名单定向清理，不误伤用户正输入的新 Key）
    private fun syncBannedKeysFromGateway() {
        if (isSyncingBanned) return
        isSyncingBanned = true

        fetchGatewayStatus { json ->
            try {
                if (json == null) return@fetchGatewayStatus
                val bannedArray = json.optJSONArray("banned_key_list") ?: return@fetchGatewayStatus
                val bannedKeys = mutableSetOf<String>()
                for (i in 0 until bannedArray.length()) {
                    val k = bannedArray.optString(i).trim()
                    if (k.isNotEmpty()) {
                        bannedKeys.add(k)
                    }
                }
                if (bannedKeys.isEmpty()) return@fetchGatewayStatus

                val localLines = etKeys.text.toString().lines()
                val filteredLines = mutableListOf<String>()
                var removedCount = 0

                for (line in localLines) {
                    val trimmed = line.trim()
                    if (trimmed.isEmpty()) continue
                    val rawKey = extractRawKey(trimmed)
                    if (bannedKeys.contains(rawKey)) {
                        removedCount++
                    } else {
                        filteredLines.add(line)
                    }
                }

                if (removedCount > 0) {
                    etKeys.setText(filteredLines.joinToString("\n"))
                    savePreferences()
                    Toast.makeText(this, "检测到 $removedCount 个 401 封禁 Key，已自动从号池移除！", Toast.LENGTH_LONG).show()
                    appendLog("[系统] 自动清理：已将 $removedCount 个失效封禁 Key 从本地号池移除并保存")
                }
            } finally {
                isSyncingBanned = false
            }
        }
    }

    private fun extractRawKey(line: String): String {
        var str = line.trim()
        val delims = listOf("----", "#", " ", "\t")
        for (d in delims) {
            val idx = str.indexOf(d)
            if (idx > 0) {
                return str.substring(0, idx).trim()
            }
        }
        val colonIdx = str.indexOf(":")
        if (colonIdx > 0 && !str.substring(0, colonIdx).contains("/")) {
            return str.substring(0, colonIdx).trim()
        }
        return str
    }

    private fun reloadKeysToRunningGateway() {
        savePreferences()
        val portStr = etPort.text.toString().trim()
        val port = portStr.toIntOrNull() ?: 8080
        val keys = etKeys.text.toString().trim()

        kotlin.concurrent.thread {
            try {
                val url = URL("http://127.0.0.1:$port/_gateway/keys")
                val conn = (url.openConnection() as HttpURLConnection).apply {
                    requestMethod = "POST"
                    doOutput = true
                    connectTimeout = 3000
                    readTimeout = 3000
                }
                conn.outputStream.use { os ->
                    os.write(keys.toByteArray(Charsets.UTF_8))
                }
                val code = conn.responseCode
                runOnUiThread {
                    if (code == 200) {
                        Toast.makeText(this, "号池已热更新生效！", Toast.LENGTH_SHORT).show()
                        appendLog("[系统] 运行中号池热重载成功！")
                        fetchGatewayStatus()
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
        tvStatus.postDelayed({
            updateUIState()
            startStatusPolling()
        }, 500)
    }

    private fun stopGatewayService() {
        val intent = Intent(this, GatewayService::class.java).apply {
            action = GatewayService.ACTION_STOP
        }
        startService(intent)
        appendLog("[系统] 正在停止网关服务...")
        tvStatus.postDelayed({
            updateUIState()
            stopStatusPolling()
        }, 500)
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
            btnReloadKeys.visibility = View.VISIBLE

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
            btnReloadKeys.visibility = View.GONE

            currentBaseUrl = ""
            tvLocalUrl.text = "http://127.0.0.1:$port/v1 (未启动)"
            tvLanUrl.text = if (localIP != null) "局域网其他设备用: http://$localIP:$port/v1" else ""
            btnCopyUrl.isEnabled = false

            tvCurrentKey.text = "当前 Key: 空闲待命"
            tvPoolHealth.text = "未启动"
            tvPoolHealth.setTextColor(0xFF9E9E9E.toInt())
            layoutAlertBanner.visibility = View.GONE
            hasNotifiedAllUnavailable = false
        }
    }

    private fun appendLog(msg: String) {
        runOnUiThread {
            // 检测 401 封禁事件，自动触发本地号池剔除同步
            if (msg.contains("[封禁]") && msg.contains("已从号池剔除")) {
                syncBannedKeysFromGateway()
            }

            val now = System.currentTimeMillis()
            logLines.addLast(now to msg)

            val pruned = pruneLogs(now)
            if (pruned) {
                rebuildLogView()
            } else {
                if (logStringBuilder.isNotEmpty()) {
                    logStringBuilder.append('\n')
                    tvLogs.append("\n")
                }
                val colored = colorizeLog(msg)
                logStringBuilder.append(colored)
                tvLogs.append(colored)
            }

            if (autoScrollEnabled) {
                tvLogs.post { scrollToBottom() }
            }

            fullscreenTvLogs?.let { fsTv ->
                if (pruned) {
                    fsTv.text = SpannableStringBuilder(logStringBuilder)
                } else {
                    if (fsTv.text.isNotEmpty()) {
                        fsTv.append("\n")
                    }
                    fsTv.append(colorizeLog(msg))
                }
                if (autoScrollEnabled) {
                    fsTv.post {
                        val layout = fsTv.layout ?: return@post
                        val bottom = layout.getLineTop(fsTv.lineCount)
                        fsTv.scrollTo(0, (bottom - fsTv.height).coerceAtLeast(0))
                    }
                }
            }
        }
    }

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

    private fun rebuildLogView() {
        logStringBuilder.clear()
        logLines.forEachIndexed { index, (_, msg) ->
            if (index > 0) logStringBuilder.append('\n')
            logStringBuilder.append(colorizeLog(msg))
        }
        tvLogs.text = SpannableStringBuilder(logStringBuilder)
    }

    // 根据简化精简后的日志结构进行行内局部高亮：时间戳灰、状态彩色加粗、耗时高亮、模型浅紫、Key青色
    private fun colorizeLog(msg: String): Spannable {
        val sp = SpannableStringBuilder(msg)

        val baseColor = when {
            msg.contains("[400]") || msg.contains("[401]") || msg.contains("[403]") ||
            msg.contains("[500]") || msg.contains("[502]") || msg.contains("[封禁]") ||
            msg.contains("[失败]") || msg.startsWith("[错误]") -> 0xFFF85149.toInt() // 红色
            msg.contains("[429]") || msg.contains("[限流]") ||
            msg.contains("[超时]") || msg.contains("[避让]") -> 0xFFFFB74D.toInt() // 橙黄色
            msg.contains("[切号]") -> 0xFF00E5FF.toInt() // 青色
            msg.contains("[中断]") -> 0xFF8B949E.toInt() // 灰色
            msg.contains("[成功]") -> 0xFF3FB950.toInt() // 鲜绿色
            msg.contains("[告警]") -> 0xFFFF1744.toInt() // 警示红
            msg.startsWith("[系统]") || msg.startsWith("[DNS]") -> 0xFF58A6FF.toInt() // 系统蓝
            msg.startsWith("===") -> 0xFF8B949E.toInt()
            else -> 0xFFE6EDF3.toInt()
        }
        sp.setSpan(ForegroundColorSpan(baseColor), 0, msg.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)

        // 1. 时间戳灰色 (HH:mm:ss 或 YYYY/MM/DD HH:mm:ss)
        PATTERN_TS.find(msg)?.let {
            sp.setSpan(ForegroundColorSpan(0xFF8B949E.toInt()), it.range.first, it.range.last + 1, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }
        PATTERN_FULL_TS.find(msg)?.let {
            sp.setSpan(ForegroundColorSpan(0xFF8B949E.toInt()), it.range.first, it.range.last + 1, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }

        // 2. 状态标签特殊高亮 [成功] / [限流] / [超时] / [避让] / [切号] / [封禁] / [失败] / [中断] / [系统] / [告警] / [xxx]
        PATTERN_TAG.findAll(msg).forEach {
            val tag = it.value
            val tagColor = when {
                tag == "[成功]" || tag.matches(PATTERN_TAG_2XX) -> 0xFF3FB950.toInt()
                tag == "[限流]" || tag == "[429]" || tag == "[超时]" || tag == "[避让]" -> 0xFFFFB74D.toInt()
                tag == "[切号]" -> 0xFF00E5FF.toInt()
                tag == "[中断]" -> 0xFF8B949E.toInt()
                tag == "[封禁]" || tag == "[失败]" || tag == "[告警]" || tag.matches(PATTERN_TAG_ERR) -> 0xFFF85149.toInt()
                else -> 0xFF58A6FF.toInt()
            }
            val start = it.range.first
            val end = it.range.last + 1
            sp.setSpan(ForegroundColorSpan(tagColor), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            sp.setSpan(StyleSpan(Typeface.BOLD), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }

        // 3. 首字响应时间 (0.xxs)
        PATTERN_TTFB.findAll(msg).forEach {
            val start = it.range.first
            val end = it.range.last + 1
            sp.setSpan(ForegroundColorSpan(0xFF80D8FF.toInt()), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            sp.setSpan(StyleSpan(Typeface.BOLD), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }

        // 4. 模型名高亮 ( | agnes-xxx | )
        PATTERN_MODEL.findAll(msg).forEach {
            val start = it.range.first + 1
            val end = it.range.last
            sp.setSpan(ForegroundColorSpan(0xFFBC8CFF.toInt()), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            sp.setSpan(StyleSpan(Typeface.BOLD), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }

        // 5. Key/备注高亮 (Key: xxx)
        PATTERN_KEY.findAll(msg).forEach {
            val start = it.range.first
            val end = it.range.last + 1
            sp.setSpan(ForegroundColorSpan(0xFF00E5FF.toInt()), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            sp.setSpan(StyleSpan(Typeface.BOLD), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        }

        return sp
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

    override fun onBackPressed() {
        if (fullscreenLogDialog?.isShowing == true) {
            fullscreenLogDialog?.dismiss()
            return
        }
        @Suppress("DEPRECATION")
        super.onBackPressed()
    }

    private fun toggleFullscreenLog() {
        if (fullscreenLogDialog?.isShowing == true) {
            fullscreenLogDialog?.dismiss()
            return
        }

        requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE

        val dialog = Dialog(this, android.R.style.Theme_Black_NoTitleBar_Fullscreen)
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(0xFF0D1117.toInt())
            setPadding(16, 16, 16, 16)
        }

        val header = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(0, 0, 0, 8)
        }

        val title = TextView(this).apply {
            text = "实时运行日志 (全屏)"
            setTextColor(0xFFE6EDF3.toInt())
            textSize = 16f
            layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)
        }

        val btnClose = Button(this).apply {
            text = "退出全屏"
            setOnClickListener { dialog.dismiss() }
        }

        val tv = TextView(this).apply {
            setBackgroundColor(0xFF0D1117.toInt())
            setTextColor(0xFFE6EDF3.toInt())
            typeface = Typeface.MONOSPACE
            textSize = 13f
            setPadding(8, 8, 8, 8)
            movementMethod = ScrollingMovementMethod()
            setTextIsSelectable(true)
            layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f)
        }
        fullscreenTvLogs = tv
        tv.text = SpannableStringBuilder(logStringBuilder)

        tv.post {
            val layout = tv.layout ?: return@post
            val bottom = layout.getLineTop(tv.lineCount)
            tv.scrollTo(0, (bottom - tv.height).coerceAtLeast(0))
        }

        header.addView(title)
        header.addView(btnClose)
        root.addView(header)
        root.addView(tv)

        dialog.setContentView(root)
        dialog.setOnDismissListener {
            fullscreenLogDialog = null
            fullscreenTvLogs = null
            requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
        }
        dialog.show()
        fullscreenLogDialog = dialog
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

    companion object {
        private val PATTERN_TS = Regex("""^\d{2}:\d{2}:\d{2}""")
        private val PATTERN_FULL_TS = Regex("""^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}""")
        private val PATTERN_TAG = Regex("""\[(成功|限流|超时|避让|切号|中断|封禁|失败|系统|告警|\d{3})\]""")
        private val PATTERN_TAG_2XX = Regex("""\[2\d{2}\]""")
        private val PATTERN_TAG_ERR = Regex("""\[[45]\d{2}\]""")
        private val PATTERN_TTFB = Regex("""\b\d+\.\d{2}s\b""")
        private val PATTERN_MODEL = Regex("""\|\s*([a-zA-Z0-9._-]+)\s*\|""")
        // 注意：字符类里的 "-" 必须放在末尾，否则 "|->" 会被解析成非法区间 (\u007C..\u003E) 导致类初始化崩溃
        private val PATTERN_KEY = Regex("""Key:\s*([^\s|>-]+)""")
    }
}
