# 会话接续状态

## 目标
修复 Agnes 手机反代网关三处问题：UI 文案、日志滚动、手机内无法解析上游域名；并把 Base URL 改为固定 127.0.0.1、接入固定 release 签名以支持覆盖升级。

## 已完成
1. 文案：activity_main.xml 副标题改为「本地号池负载均衡 · 局域网极速反向代理」（去掉 Cloudflare 说法）；README 相关表述同步。
2. 日志滚动：tvLogs 设 ScrollingMovementMethod，appendLog 后在 tvLogs.post 中按 lineCount 计算底部并 scrollTo，日志自动跟随最新行。
3. 手机连不上根因修复：Go 子进程在 Android 读不到 resolv.conf（报 lookup agnes-ai.cn on [::1]:53: connection refused）。
   - Kotlin：MainActivity.getSystemDnsServers() 用 ConnectivityManager 读取当前网络 DNS，经 EXTRA_DNS → --dns= 传给内核。
   - Go：新增 normalizeDNSServer / newResolver / probeDNSServers，customResolver 注入 net.Dialer.Resolver；候选 = App 传入 DNS + 223.5.5.5 / 119.29.29.29 / 114.114.114.114 / 8.8.8.8，并行探测 2s 选可用。
   - 单测：TestNormalizeDNSServer；go test ./... + go vet ./... 全绿；arm64 二进制已重新交叉编译并同步到 jniLibs/arm64-v8a/libgateway.so。
4. Base URL：主显示固定 http://127.0.0.1:<port>/v1（currentBaseUrl 供复制按钮用），新增 tvLanUrl 灰字提示「局域网其他设备用: http://<局域网IP>:<port>/v1」。
5. 热更新号池：Key 输入框下补充说明文案。
6. 固定签名：keystore/agnes-release.p12（PKCS12，2048 RSA，30 年有效期，SHA256=a5b0b094b1ccc327cab2eb7504bfcf22f36e34e56cf5411e6c96ff69717ef21a）。app/build.gradle.kts 支持环境变量（CI）与 keystore/keystore.properties（本地）；/keystore/ 已 gitignore；workflow 增加 Decode Release Keystore + assembleRelease。
7. 版本号：versionCode 2 / versionName 1.0.1（v1.0.1）。

## 未完成 / 下一步
- 用户需在 GitHub 仓库 Secrets 添加（否则 CI 构建失败）：
  KEYSTORE_BASE64（keystore/agnes-release.p12.b64 全文）、KEYSTORE_PASSWORD、KEY_ALIAS=agnes-release、KEY_PASSWORD（与 keystore 密码相同，见 keystore/keystore.properties）。
- 加完 Secret 后：push main → CI 出 AgnesGateway-v1.0.1.apk → 打 tag v1.0.1 发布 Release。
- 旧 v1.0.0 是 CI 临时 debug 证书签名，与固定签名不兼容，本次需卸载旧版再装，此后可覆盖升级。
- 真机验证项：启动后日志应出现 [DNS] 可用 DNS 服务器: ...；手机内 Agent 用 127.0.0.1 调通。

## 关键文件
- core/main.go：DNS 解析器装配（normalizeDNSServer / newResolver / probeDNSServers / main 中 --dns 处理）
- core/main_test.go：TestNormalizeDNSServer
- app/src/main/java/com/agnes/gateway/MainActivity.kt：DNS 读取、127.0.0.1 展示、日志滚动
- app/src/main/java/com/agnes/gateway/GatewayService.kt：EXTRA_DNS → --dns 透传
- app/src/main/res/layout/activity_main.xml：文案、tvLanUrl、号池说明
- app/build.gradle.kts：固定签名配置、versionCode/Name
- .github/workflows/build-apk.yml：keystore 解码 + assembleRelease + 发布说明
- keystore/（gitignored）：agnes-release.p12 / .p12.b64 / keystore.properties / credentials.txt

## 佐证
- go test ./... → ok agnes-gateway-core；go vet ./... 无输出。
- Python cryptography 验证：p12 可解出 2048 位私钥，证书 SHA256 与 credentials.txt 一致；base64 与 p12 二进制一致。
- workflow YAML 解析通过，步骤含 Decode Release Keystore / Build Signed Release APK。
- 环境限制：本机无 JDK/Android SDK，无法本地 assembleRelease，APK 由 GitHub Actions 产出。
