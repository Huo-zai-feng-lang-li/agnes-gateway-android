# Agnes 手机反代网关 (Agnes Gateway for Android)

> 一个跑在安卓手机上的 **本地局域网反向代理网关**：手机进程内起 HTTP 服务，聚合 Agnes 号池，对外提供标准 OpenAI 兼容接口。  
> 利用闲置安卓手机的 **5G/4G 蜂窝网络** 或家庭宽带出口，局域网内任何设备（手机里的 Agent、电脑上的客户端）都能直接调用。

---

## 🌟 核心特性

- **移动网络加速**：利用手机端移动/联通/电信 5G 蜂窝网络的纯净公网 IP 与低延迟出口，跳过机房 IP 限制与劣质 Anycast 路由。
- **号池负载均衡**：支持批量配置多个 Agnes API Key，自动平滑轮询（Round-Robin）。
- **429 智能退避与自愈**：单个 Key 遭遇 429 限流时自动冷却（默认 120 秒），并**无感重试切换到下一个可用 Key**；遇到 401 自动禁用失效 Key。
- **锁屏后台持续保活**：基于 Android 原生前台服务（Foreground Service）+ CPU `WakeLock` + `WifiLock`，防止手机熄屏休眠断流。
- **极速流式透传**：上游 HTTP/2 长连接复用（Keep-Alive），流式 SSE 逐块实时 flush，杜绝卡顿。
- **自动 DNS 适配**：App 读取当前网络的系统 DNS 注入内核，内核自建解析器并并行探测可用 DNS（含国内公共 DNS 兜底），彻底规避安卓子进程 `/etc/resolv.conf` 缺失导致的 `lookup ... :53: connection refused`。
- **双地址提示**：主 URL 固定为 `http://127.0.0.1:8080/v1`（手机内的 Agent 填这个，换网不用改），另附局域网地址供其他设备使用，均支持一键复制。

> 📌 **当前版本：v1.0.10**。完整变更历史见 [CHANGELOG.md](CHANGELOG.md)。

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
3. **配置固定签名密钥（首次必须）**：

   CI 产出的 APK 使用仓库 Secrets 中的固定 keystore 签名，保证每次升级都能覆盖安装。首次需在仓库 `Settings → Secrets and variables → Actions` 添加：

   | Secret 名 | 内容 |
   | --- | --- |
   | `KEYSTORE_BASE64` | `keystore/agnes-release.p12.b64` 的全部内容 |
   | `KEYSTORE_PASSWORD` | keystore 密码 |
   | `KEY_ALIAS` | `agnes-release` |
   | `KEY_PASSWORD` | key 密码（通常与 keystore 密码一致） |

   > ⚠️ 该 keystore 一旦丢失，后续版本将无法覆盖升级，请务必本地离线备份 `keystore/` 目录。
   > 早期 v1.0.0 使用 CI 临时 debug 证书，**与固定签名不兼容，需卸载一次旧版再安装**。

4. **打 tag 触发正式发布（才会生成 Release）**：
   ```bash
   git tag v1.0.10
   git push origin v1.0.10
   ```
   > 只有推送 `v*` 形式的 tag 才会构建签名 APK 并创建 GitHub Release；仅 `push main` 只上传 Artifact 且**不创建 Release**。

5. **下载 APK 安装包**：
   - **正式版（推荐）**：打开该 GitHub 仓库的 **Releases** 页面，下载最新 Release 中的 `AgnesGateway-v1.0.10.apk`，即签名可覆盖升级的安装包。
   - **开发版**：在 **Actions** 标签页点开 `Build Android APK` 运行记录，页面底部的 **Artifacts** 区域可下载 `AgnesGateway-APK.zip`（仅 `push main` 时为 `AgnesGateway-dev-<提交短哈希>.apk`）。

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

网关启动后，界面上会同时给出两个地址：

| 场景 | Base URL |
| --- | --- |
| 手机本机内的 Agent / App（换网不变，推荐） | `http://127.0.0.1:8080/v1` |
| 局域网内其他设备（电脑、另一台手机） | `http://192.168.x.x:8080/v1` |

### 1. 直连客户端（Cline / Cherry Studio / NextChat 等）
- **API 接口地址 (Base URL)**: 同机填 `http://127.0.0.1:8080/v1`，跨设备填 `http://192.168.x.x:8080/v1`
- **API Key**: 随意填写（例如 `123456`，手机网关已负责向 Agnes 注入有效 Key）
- **模型名**: 直接填 Agnes 支持的模型（如 `gpt-6-astra`, `agnes-2.5-flash` 等）

### 2. 作为上游接入现有 `codex2api`
如果你电脑上的 `codex2api` 负责管理全套官方 Codex 账号，你可以把这台手机当成一个**高性能自建聚合上游**：
- 在 `codex2api` 的系统设置中添加一个账号：
  - **Base URL**: `http://192.168.x.x:8080/v1`（与手机同机则可用 `http://127.0.0.1:8080/v1`）
  - **API Key**: `123456`
- 此时发往 Agnes 的请求将自动通过手机网络出口转发，规避机房 IP 与劣质路由。

### 3. 运行中热更新号池
网关运行中直接编辑界面的 Key 输入框，再点右上角 **「热更新号池」**，新 Key 立即生效（`POST /_gateway/keys`），无需重启、不中断在途请求。

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
│   │   │   └── libgateway.so       # 原生执行权限绑定的 Go 核心二进制（libgateway.so 实为 Go 可执行文件）
│   │   ├── java/com/agnes/gateway/
│   │   │   ├── MainActivity.kt     # 质感 UI、参数持久化、实时滚动日志
│   │   │   ├── GatewayService.kt   # 前台常驻保活服务、进程守护
│   │   │   └── NetworkUtils.kt     # 局域网 IP 自动获取
│   │   └── res/                    # 布局、配色、图标资源
│   └── ...
├── keystore/                       # 本地签名密钥（已 gitignore，勿提交）
│   ├── agnes-release.p12           # PKCS12 固定 release keystore
│   ├── agnes-release.p12.b64       # 供 GitHub Secret 使用的 base64
│   └── keystore.properties         # 本地构建读取的签名配置
├── build.gradle.kts
├── settings.gradle.kts
└── README.md
```
