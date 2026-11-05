package com.agnes.gateway

import android.annotation.SuppressLint
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat

class MainActivity : AppCompatActivity() {

    private lateinit var tvStatus: TextView
    private lateinit var tvLocalUrl: TextView
    private lateinit var btnCopyUrl: Button
    private lateinit var etUpstream: EditText
    private lateinit var etPort: EditText
    private lateinit var etKeys: EditText
    private lateinit var btnToggle: Button
    private lateinit var btnBatteryOpt: Button
    private lateinit var tvLogs: TextView
    private lateinit var btnClearLog: Button

    private val logStringBuilder = StringBuilder()

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
        btnCopyUrl = findViewById(R.id.btnCopyUrl)
        etUpstream = findViewById(R.id.etUpstream)
        etPort = findViewById(R.id.etPort)
        etKeys = findViewById(R.id.etKeys)
        btnToggle = findViewById(R.id.btnToggle)
        btnBatteryOpt = findViewById(R.id.btnBatteryOpt)
        tvLogs = findViewById(R.id.tvLogs)
        btnClearLog = findViewById(R.id.btnClearLog)

        btnCopyUrl.setOnClickListener {
            val url = tvLocalUrl.text.toString()
            if (url.isNotBlank() && url != "未启动") {
                val clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                val clip = ClipData.newPlainText("Agnes Gateway URL", url)
                clipboard.setPrimaryClip(clip)
                Toast.makeText(this, "已复制 Base URL 到剪贴板", Toast.LENGTH_SHORT).show()
            }
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
            tvLogs.text = ""
        }
    }

    private fun startGatewayService() {
        savePreferences()
        val portStr = etPort.text.toString().trim()
        val port = portStr.toIntOrNull() ?: 8080
        val upstream = etUpstream.text.toString().trim().ifEmpty { "https://agnes-ai.cn" }
        val keys = etKeys.text.toString().trim()

        if (keys.isEmpty()) {
            Toast.makeText(this, "提示: 当前未配置 Agnes Key，请先输入 Key 号池", Toast.LENGTH_LONG).show()
        }

        val intent = Intent(this, GatewayService::class.java).apply {
            action = GatewayService.ACTION_START
            putExtra(GatewayService.EXTRA_PORT, port)
            putExtra(GatewayService.EXTRA_UPSTREAM, upstream)
            putExtra(GatewayService.EXTRA_KEYS, keys)
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

            val fullUrl = if (localIP != null) "http://$localIP:$port/v1" else "http://127.0.0.1:$port/v1"
            tvLocalUrl.text = fullUrl
            btnCopyUrl.isEnabled = true
        } else {
            tvStatus.text = "○ 已停止 (Stopped)"
            tvStatus.setTextColor(0xFF9E9E9E.toInt()) // 灰色
            btnToggle.text = "启动反代网关"
            btnToggle.setBackgroundColor(0xFF2979FF.toInt()) // 蓝色
            etPort.isEnabled = true
            etUpstream.isEnabled = true

            tvLocalUrl.text = if (localIP != null) "http://$localIP:$port/v1 (未启动)" else "未启动"
            btnCopyUrl.isEnabled = false
        }
    }

    private fun appendLog(msg: String) {
        runOnUiThread {
            logStringBuilder.append(msg).append("\n")
            if (logStringBuilder.length > 20000) {
                logStringBuilder.delete(0, 5000)
            }
            tvLogs.text = logStringBuilder.toString()
        }
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

    private fun loadPreferences() {
        val sp = getSharedPreferences("gateway_config", Context.MODE_PRIVATE)
        etUpstream.setText(sp.getString("upstream", "https://agnes-ai.cn"))
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
