# Agnes 手机反代网关 (Agnes Gateway for Android)

> 专为解决国内直连 **Cloudflare (CF) / Agnes AI** 接口延迟高、握手慢、频繁断流而设计的安卓独立反代网关。  
> 利用闲置安卓手机的 **5G/4G 蜂窝网络优质路由** 或家庭宽带，本地聚合 Agnes 号池，对外提供标准 OpenAI 兼容接口。

---

## 🌟 核心特性

- **移动网络加速**：利用手机端移动/联通/电信 5G 蜂窝网络的纯净公网 IP 与低延迟出口，跳过机房 IP 限制与劣质 Anycast 路由。
- **号池负载均衡**：支持批量配置多个 Agnes API Key，自动平滑轮询（Round-Robin）。
- **429 智能退避与自愈**：单个 Key 遭遇 429 限流时自动冷却（默认 120 秒），并**无感重试切换到下一个可用 Key**；遇到 401 自动禁用失效 Key。
- **锁屏后台持续保活**：基于 Android 原生前台服务（Foreground Service）+ CPU `WakeLock` + `WifiLock`，防止手机熄屏休眠断流。
- **极速流式透传**：深度定制针对 Cloudflare 的长连接池复用（HTTP Keep-Alive），流式 SSE 逐块实时 flush，杜绝卡顿。
- **零配置开箱即用**：自动识别手机局域网 IPv4 地址（如 `http://192.168.1.100:8080/v1`），带一键复制功能。

---

## 🚀 两种使用姿态

### 姿态一：一键推送到 GitHub 自动打包成 APK 安装包（推荐）

本项目已预置完整的 **GitHub Actions 自动化流水线**（`.github/workflows/build-apk.yml`），无需在电脑上配置繁重的 Android SDK / Java 环境！

1. **新建 GitHub 仓库**：在 GitHub 上创建一个新仓库（如 `agnes-gateway`）。
2. **推送本工程代码**：
   ```bash
   cd D:\Desktop\agnes-gateway-android
   git init
   git add .
   git commit -m "feat: initial commit for agnes gateway android"
   git branch -M main
   git remote add origin https://github.com/<你的用户名>/<你的仓库名>.git
   git push -u origin main
   ```
3. **下载 APK 安装包**：
   - 打开该 GitHub 仓库的 **Actions** 标签页；
   - 点击刚刚运行的构建任务 `Build Android APK`；
   - 在页面底部的 **Artifacts** 区域，直接下载打包好的 `AgnesGateway-v1.0.0-APK.zip`，解压即得可以直接安装在手机上的 APK！

---

### 姿态二：Termux 免打包 5 分钟极速运行（已编译好 ARM64 二进制）

本工程的 `core/` 目录中已经**为你直接交叉编译好了** Android 64 位原生二进制文件：`core/agnes-gateway-arm64`（体积仅 6MB，纯 Go 无 CGO 依赖）。

如果你不想打包 APK，只需要 3 步就能立刻在手机上跑起来测速：

1. **手机安装 Termux**：从 F-Droid 或应用商店下载安装 [Termux](https://f-droid.org/zh_Hans/packages/com.termux/)。
2. **将文件传到手机**：
   - 通过微信/QQ/数据线将 `core/agnes-gateway-arm64` 发送到手机存储。
3. **在 Termux 中运行**：
   ```bash
   # 进入文件存放目录（或直接从手机 Download 目录复制）
   cp /sdcard/Download/agnes-gateway-arm64 ./
   chmod +x ./agnes-gateway-arm64

   # 启动网关 (填入你的多个 Agnes Key，逗号分隔)
   ./agnes-gateway-arm64 --port=8080 --keys="sk-agnes-xxx1,sk-agnes-xxx2"
   ```
4. 终端会打印出：
   ```
   [号池] 已加载 2 个有效 Key
   网关服务已启动，局域网监听地址: http://0.0.0.0:8080/v1
   ```

---

## 📱 客户端配置与对接指引

当手机上的网关启动后，界面（或日志）会显示局域网 Base URL：
`http://192.168.x.x:8080/v1`

### 1. 直连客户端（Cline / Cherry Studio / NextChat 等）
- **API 接口地址 (Base URL)**: `http://192.168.x.x:8080/v1`
- **API Key**: 随意填写（例如 `123456`，手机网关已负责向 Agnes 注入有效 Key）
- **模型名**: 直接填 Agnes 支持的模型（如 `gpt-6-astra`, `agnes-2.5-flash` 等）

### 2. 作为上游接入现有 `codex2api`
如果你电脑上的 `codex2api` 负责管理全套官方 Codex 账号，你可以把这台手机当成一个**高性能自建聚合上游**：
- 在 `codex2api` 的系统设置中添加一个账号：
  - **Base URL**: `http://192.168.x.x:8080/v1`
  - **API Key**: `123456`
- 此时发往 Agnes 的请求将自动通过手机网络路由出海，彻底消除 CF 的网络波动与高延迟。

---

## ⚠️ 手机做网关的注意事项（防杀后台指南）

1. **开启电池优化白名单**：
   - 首次打开 App，点击界面上的 **「后台防杀保活」** 按钮，在系统弹窗中允许“忽略电池优化”（无限制后台运行）。
2. **多任务界面锁定**：
   - 在手机的多任务卡片界面，长按本应用卡片，点击 **加锁（小锁图标）**，防止被系统一键清理杀后台。
3. **绑定路由器静态 IP**：
   - 建议在家里路由器的 DHCP 静态绑定中，将手机的 MAC 地址固定为一个固定的 IP（例如 `192.168.1.188`），避免每次重连 WiFi 后 IP 改变。

---

## 🛠️ 项目工程结构

```
agnes-gateway-android/
├── .github/workflows/
│   └── build-apk.yml               # GitHub Actions 自动化构建 APK 流水线
├── core/                           # Go 核心反代微服务
│   ├── go.mod
│   ├── main.go                     # 高性能号池管理、429退避、SSE流式透传
│   ├── build_arm64.bat             # 本地 Windows 交叉编译 ARM64 脚本
│   ├── run_termux.sh               # Termux 启动脚本
│   └── agnes-gateway-arm64         # [已生成] Android 64 位原生可执行文件
├── app/                            # Android 原生应用壳
│   ├── build.gradle.kts
│   ├── src/main/
│   │   ├── AndroidManifest.xml     # 前台服务、WakeLock 等权限配置
│   │   ├── jniLibs/arm64-v8a/
│   │   │   └── libgateway.so       # 原生执行权限绑定的 Go 核心二进制
│   │   ├── java/com/agnes/gateway/
│   │   │   ├── MainActivity.kt     # 质感 UI、参数持久化、实时滚动日志
│   │   │   ├── GatewayService.kt   # 前台常驻保活服务、进程守护
│   │   │   └── NetworkUtils.kt     # 局域网 IP 自动获取
│   │   └── res/                    # 布局、配色、图标资源
│   └── ...
├── build.gradle.kts
├── settings.gradle.kts
└── README.md
```
