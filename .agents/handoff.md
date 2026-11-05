# 会话接续状态

## 目标
修复 Agnes 手机反代网关三处问题：UI 文案、日志滚动、手机内无法解析上游域名；并把 Base URL 改为固定 127.0.0.1、接入固定 release 签名以支持覆盖升级。本次会话完成收尾发版：递增到 v1.0.2 并推送 tag、补 CHANGELOG、修复 workflow 版本号硬编码缺陷。

## 已完成（本次会话新增）
8. 排查发版状态：本地 main(f2ac739) 含 DNS 修复但未推送；远程 tag v1.0.1 指向 0b7c41b（versionName 仍为 1.0.0），CI 已用旧内容产出 Release，修复内容实际未发布。
9. **修复 workflow 版本号硬编码缺陷**（根因）：原 `.github/workflows/build-apk.yml` 中 `TAG="${{ startsWith(github.ref,'refs/tags/') && github.ref_name || 'v1.0.1' }}"` 硬编码兜底，导致 push main 时就用 v1.0.1 创建/覆盖 Release。现改为：
   - Rename 步骤：tag 触发用 $GITHUB_REF_NAME，非 tag 用 dev-${GITHUB_SHA::7}；
   - Release 步骤加 `if: startsWith(github.ref, 'refs/tags/v')`，`tag_name`/`name` 取自 `github.ref_name`。
10. 版本号升至 versionCode 3 / versionName 1.0.2。
11. 新建 CHANGELOG.md（v1.0.0 / v1.0.1 / v1.0.2 三段，含 v1.0.1 版本号未递增的说明）。
12. README：APK 文件名改 v1.0.2；核心特性后加"当前版本 v1.0.2 + CHANGELOG 链接"。
13. YAML 语法校验通过（python yaml.safe_load）。

## 未完成 / 下一步
- 提交并 push main（f2ac739 之后的新提交）；
- 打 tag v1.0.2 并 push tag，触发 CI 出签名 APK + GitHub Release；
- 推送后核对：Release v1.0.2 的 asset 为 AgnesGateway-v1.0.2.apk，且 push main 那次运行**不再**创建 Release（只出 Artifact，名 AgnesGateway-dev-<sha7>.apk）。
- 真机验证项（用户执行）：卸载旧 v1.0.0/v1.0.1 后安装新 APK（签名切换）；启动后日志应出现 `[DNS] 可用 DNS 服务器: ...`；手机内 Agent 用 127.0.0.1 调通。

## 关键文件
- .github/workflows/build-apk.yml：keystore 解码 + assembleRelease + 条件发布（仅 tag）
- CHANGELOG.md：版本变更记录（新增）
- app/build.gradle.kts：versionCode 3 / versionName 1.0.2、固定签名配置
- core/main.go / core/main_test.go：DNS 解析器装配与单测
- app/src/main/java/com/agnes/gateway/MainActivity.kt：DNS 读取、127.0.0.1 展示、日志滚动
- app/src/main/java/com/agnes/gateway/GatewayService.kt：EXTRA_DNS → --dns 透传
- app/src/main/res/layout/activity_main.xml：文案、tvLanUrl、号池说明
- keystore/（gitignored）：agnes-release.p12 / .p12.b64 / keystore.properties / credentials.txt

## 佐证
- go test ./... → ok agnes-gateway-core；go vet ./... 无输出。
- core/agnes-gateway-arm64 与 app/src/main/jniLibs/arm64-v8a/libgateway.so SHA256 一致：503f857d4b8a9bb39e16c8a8b80a6371067be77f9929838be4634a6cfff2c2bf。
- workflow YAML 校验通过。
- gh（经 http://127.0.0.1:51081 代理）可查 Release：v1.0.1 / v1.0.0 均存在。
- 环境限制：本机无 JDK/Android SDK（java 不在 PATH），无法本地 assembleRelease，APK 由 GitHub Actions 产出。
