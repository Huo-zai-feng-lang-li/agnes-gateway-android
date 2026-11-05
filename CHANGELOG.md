# 更新日志

本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [v1.0.2] - 2026-11-06

### 修复
- **手机内进程无法解析域名**：Go 子进程在 Android 上读不到 `resolv.conf`，报 `lookup agnes-ai.cn on [::1]:53: connection refused`。
  - Kotlin 侧新增 `getSystemDnsServers()`，通过 `ConnectivityManager` 读取当前网络的 DNS，经 `EXTRA_DNS` → `--dns=` 注入内核。
  - Go 侧新增 `normalizeDNSServer` / `newResolver` / `probeDNSServers`，自建 `net.Resolver` 注入 `net.Dialer.Resolver`；候选 DNS = App 传入 + 223.5.5.5 / 119.29.29.29 / 114.114.114.114 / 8.8.8.8，并行探测 2s 择优。
  - 新增单测 `TestNormalizeDNSServer`。

### 变更
- **Base URL 固定为 `http://127.0.0.1:<port>/v1`**：换网络无需改配置；局域网地址单独以灰字提示「局域网其他设备用: http://<局域网IP>:<port>/v1」。
- 实时日志支持滚动，并在追加后自动跟随到最新一行。
- 界面副标题修正为「本地号池负载均衡 · 局域网极速反向代理」，去掉 Cloudflare 表述；Key 输入框下补充热更新号池说明。
- 接入固定 release 签名（PKCS12 / RSA 2048 / SHA256），后续版本可直接覆盖升级。
- 版本号 `versionCode 3` / `versionName 1.0.2`。

### 构建 / CI
- **修复 workflow 版本号硬编码缺陷**：原先 `push main` 时兜底使用 `v1.0.1` 去创建/覆盖 Release，导致未打 tag 就产出 Release，且后续每次 push 都会覆盖同一个 Release。现在：
  - `push main` 只上传 Artifact（文件名 `AgnesGateway-dev-<sha7>.apk`），不再创建 Release；
  - 仅 `push tag v*` 触发 Release，`tag_name` / `name` 取自实际 tag。

### 验证
- `go test ./...` → ok；`go vet ./...` → 无输出。
- `core/agnes-gateway-arm64` 与 `app/src/main/jniLibs/arm64-v8a/libgateway.so` SHA256 一致（`503f857d...`）。
- CI workflow YAML 语法校验通过。

> ⚠️ 早期 v1.0.0 使用 CI 临时 debug 证书签名，**与固定签名不兼容**，首次升级到固定签名版本需卸载旧版再安装，此后可覆盖升级。

## [v1.0.1] - 2026-11-05

> 注：该 tag 指向的提交 `versionName` 仍为 `1.0.0`（版本号未随之递增），故本次以 v1.0.2 发布实际修复内容。

### 修复
- 解决小米 HyperOS / MIUI 及高版本 Android 上找不到 `libgateway.so` 的问题：增加 `android:extractNativeLibs="true"`，并增加从 APK 自动自解压核心引擎的兜底机制。

### 变更
- 接入 GitHub Actions 自动发布 Release（`contents: write`）。

## [v1.0.0] - 2026-11-05

### 新增
- 首个版本：Agnes 手机反代网关。
- 多 Key 轮询与 30s 智能退避重试，支持多轮工具调用透传。
- 前台保活与自愈：Notification + WakeLock，子进程崩溃由看门狗自动拉起。
- 运行中热重载：支持免重启热更新 Key 号池。
- 局域网直接调用 `http://<手机IP>:8080/v1`。

[v1.0.2]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.2
[v1.0.1]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.1
[v1.0.0]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.0
