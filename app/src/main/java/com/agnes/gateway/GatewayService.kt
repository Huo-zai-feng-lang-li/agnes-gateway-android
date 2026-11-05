package com.agnes.gateway

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.net.wifi.WifiManager
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import androidx.core.app.NotificationCompat
import java.io.BufferedReader
import java.io.File
import java.io.InputStreamReader
import kotlin.concurrent.thread

class GatewayService : Service() {

    private var wakeLock: PowerManager.WakeLock? = null
    private var wifiLock: WifiManager.WifiLock? = null
    private var process: Process? = null

    companion object {
        const val CHANNEL_ID = "agnes_gateway_channel"
        const val NOTIFICATION_ID = 1001

        const val ACTION_START = "com.agnes.gateway.START"
        const val ACTION_STOP = "com.agnes.gateway.STOP"
        const val ACTION_LOG_BROADCAST = "com.agnes.gateway.LOG"

        const val EXTRA_PORT = "extra_port"
        const val EXTRA_UPSTREAM = "extra_upstream"
        const val EXTRA_KEYS = "extra_keys"
        const val EXTRA_DNS = "extra_dns"
        const val EXTRA_LOG_LINE = "extra_log_line"

        var isRunning: Boolean = false
            private set
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        acquireLocks()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action ?: ACTION_START
        if (action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }

        val port = intent?.getIntExtra(EXTRA_PORT, 8080) ?: 8080
        val upstream = intent?.getStringExtra(EXTRA_UPSTREAM) ?: "https://agnes-ai.cn"
        val keys = intent?.getStringExtra(EXTRA_KEYS) ?: ""
        val dns = intent?.getStringExtra(EXTRA_DNS) ?: ""

        startForeground(NOTIFICATION_ID, buildNotification(port))
        isRunning = true

        startGatewayProcess(port, upstream, keys, dns)
        return START_STICKY
    }

    private fun acquireLocks() {
        try {
            val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
            wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "AgnesGateway::WakeLock").apply {
                acquire()
            }

            val wm = applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
            wifiLock = wm.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "AgnesGateway::WifiLock").apply {
                acquire()
            }
        } catch (_: Exception) {
        }
    }

    private fun releaseLocks() {
        try {
            if (wakeLock?.isHeld == true) wakeLock?.release()
            if (wifiLock?.isHeld == true) wifiLock?.release()
        } catch (_: Exception) {
        }
    }

    private fun ensureBinaryReady(): File? {
        val targetBinary = File(filesDir, "libgateway")
        val nativeLibDir = applicationInfo.nativeLibraryDir
        val sysBinary = File(nativeLibDir, "libgateway.so")

        // 1. 若系统成功解压到 nativeLibraryDir，直接优先使用
        if (sysBinary.exists() && sysBinary.length() > 0) {
            sysBinary.setExecutable(true, false)
            broadcastLog("[系统] 定位到系统原生库: ${sysBinary.absolutePath}")
            return sysBinary
        }

        // 2. 若 filesDir 中已存在且完整，直接复用
        if (targetBinary.exists() && targetBinary.length() > 1024 * 1024) {
            targetBinary.setExecutable(true, false)
            targetBinary.setReadable(true, false)
            broadcastLog("[系统] 定位到已释放核心: ${targetBinary.absolutePath}")
            return targetBinary
        }

        // 3. 从 APK 包中自解压 lib/arm64-v8a/libgateway.so 到 filesDir
        broadcastLog("[系统] 系统未解压 native 库，正在从 APK 提取核心引擎...")
        try {
            val apkFile = File(applicationInfo.sourceDir)
            java.util.zip.ZipFile(apkFile).use { zip ->
                val entry = zip.getEntry("lib/arm64-v8a/libgateway.so")
                    ?: zip.entries().asSequence().firstOrNull { it.name.endsWith("libgateway.so") }

                if (entry == null) {
                    broadcastLog("[错误] APK 中未检索到 libgateway.so！")
                    return null
                }

                zip.getInputStream(entry).use { input ->
                    targetBinary.outputStream().use { output ->
                        input.copyTo(output)
                    }
                }
            }

            targetBinary.setExecutable(true, false)
            targetBinary.setReadable(true, false)
            broadcastLog("[系统] 核心引擎自解压成功: ${targetBinary.length() / 1024} KB")
            return targetBinary
        } catch (e: Exception) {
            broadcastLog("[错误] 从 APK 提取核心失败: ${e.message}")
            return null
        }
    }

    private fun startGatewayProcess(port: Int, upstream: String, keys: String, dns: String) {
        thread(isDaemon = true) {
            try {
                val binaryFile = ensureBinaryReady()
                if (binaryFile == null || !binaryFile.exists()) {
                    broadcastLog("[错误] 核心引擎定位失败，请检查安装包完整性！")
                    return@thread
                }

                val cmdList = mutableListOf(
                    binaryFile.absolutePath,
                    "--port=$port",
                    "--upstream=$upstream"
                )
                if (keys.isNotBlank()) {
                    // 替换回车为逗号
                    val formattedKeys = keys.lines()
                        .map { it.trim() }
                        .filter { it.isNotEmpty() }
                        .joinToString(",")
                    cmdList.add("--keys=$formattedKeys")
                }
                if (dns.isNotBlank()) {
                    cmdList.add("--dns=$dns")
                }

                broadcastLog("[系统] 启动命令: ${cmdList.joinToString(" ")}")
                val processBuilder = ProcessBuilder(cmdList)
                processBuilder.redirectErrorStream(true)

                // 注入环境
                val env = processBuilder.environment()
                env["TMPDIR"] = cacheDir.absolutePath

                val proc = processBuilder.start()
                process = proc

                val reader = BufferedReader(InputStreamReader(proc.inputStream))
                var line: String?
                while (reader.readLine().also { line = it } != null) {
                    broadcastLog(line ?: "")
                }

                val exitCode = proc.waitFor()
                broadcastLog("[系统] 核心网关进程已退出，退出码: $exitCode")

                // 如果服务未被手动停止且非正常退出，启动看门狗自愈
                if (isRunning && exitCode != 0) {
                    broadcastLog("[守护] 进程异常退出，2秒后尝试自动拉起...")
                    Thread.sleep(2000)
                    if (isRunning) {
                        startGatewayProcess(port, upstream, keys, dns)
                        return@thread
                    }
                }
            } catch (e: Exception) {
                broadcastLog("[错误] 启动核心网关异常: ${e.message}")
            } finally {
                // 如果并非自动重启链路，标记停止并释放硬件锁
                if (!isRunning) {
                    releaseLocks()
                }
            }
        }
    }

    private fun broadcastLog(msg: String) {
        val intent = Intent(ACTION_LOG_BROADCAST)
        intent.putExtra(EXTRA_LOG_LINE, msg)
        intent.setPackage(packageName)
        sendBroadcast(intent)
    }

    private fun buildNotification(port: Int): Notification {
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val localIP = NetworkUtils.getLocalIPAddress() ?: "127.0.0.1"
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("Agnes 手机反代网关运行中")
            .setContentText("局域网调用接口: http://$localIP:$port/v1")
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "Agnes 网关常驻通知",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "保持 Agnes 手机反代网关在后台持续运行"
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(channel)
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        isRunning = false
        try {
            process?.destroyForcibly()
        } catch (_: Exception) {
        }
        releaseLocks()
        broadcastLog("[系统] 前台服务已停止，WakeLock 已释放")
    }
}
