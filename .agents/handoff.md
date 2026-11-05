# 会话接续状态

## 目标
修复 Agnes 手机反代网关三处问题：UI 文案、日志滚动、手机内无法解析上游域名；并把 Base URL 改为固定 127.0.0.1、接入固定 release 签名以支持覆盖升级。已发布 v1.0.2（修复 workflow 版本号硬编码）。v1.0.3 修复上游根地址错误：原默认值 https://agnes-ai.cn 是官网首页，真实 API 根地址为 https://apihub.agnes-ai.cn/v1。v1.0.7 日志行内局部高亮：时间戳灰、状态码按段分色加粗、Key 青色加粗。

## 已完成（本次会话新增）
8. 排查发版状态：本地 main(f2ac739) 含 DNS 修复但未推送；远程 tag v1.0.1 指向 0b7c41b（versionName 仍为 1.0.0），CI 已用旧内容产出 Release，修复内容实际未发布。
9. **修复 workflow 版本号硬编码缺陷**（根因）：原 `.github/workflows/build-apk.yml` 中 `TAG="${{ startsWith(github.ref,'refs/tags/') && github.ref_name || 'v1.0.1' }}"` 硬编码兜底，导致 push main 时就用 v1.0.1 创建/覆盖 Release。现改为：
   - Rename 步骤：tag 触发用 $GITHUB_REF_NAME，非 tag 用 dev-${GITHUB_SHA::7}；
   - Release 步骤加 `if: startsWith(github.ref, 'refs/tags/v')`，`tag_name`/`name` 取自 `github.ref_name`。
10. 版本号升至 versionCode 3 / versionName 1.0.2。
11. 新建 CHANGELOG.md（v1.0.0 / v1.0.1 / v1.0.2 三段，含 v1.0.1 版本号未递增的说明）。
12. README：APK 文件名改 v1.0.2；核心特性后加"当前版本 v1.0.2 + CHANGELOG 链接"。
13. YAML 语法校验通过（python yaml.safe_load）。
14. 子代理 code-reviewer 审查后修正三处：README 姿态一补「打 tag」步骤（原第 4 步与 push main 行为矛盾）；workflow tag 过滤收紧为 `v[0-9]*`；Release 正文改为引用 CHANGELOG 的通用模板（不再硬编码版本文案）。
15. 提交 f99c9de 并推送 main；打附注 tag v1.0.2 并推送。
16. **CI 首次失败两次，根因是 Secret 配置问题（已修复）**：
   - 第一次：`KEYSTORE_BASE64` 为空 → 说明 f2ac739 加签名校验后 Secret 从未配置；
   - 用 `echo %VAR% | gh secret set` 设置后第二次失败：`keystore password was incorrect`，原因是 cmd 的 echo 把尾随空格一并写入 secret（KEYSTORE_PASSWORD/KEY_ALIAS/KEY_PASSWORD 全部被污染）；
   - 改用 `gh secret set NAME --body "value"` 重设三个 Secret 后成功；
   - 注意：环境变量 `GITHUB_TOKEN` 会抢占 gh 认证导致 403，设置 Secret 前需 `set GITHUB_TOKEN=` 清除。
17. **发版成功**：Release v1.0.2 已发布，资产 `AgnesGateway-v1.0.2.apk`；push main 那次运行 success 且**未**创建 Release，artifact 名为 `AgnesGateway-APK`（内含 `AgnesGateway-dev-f99c9de.apk`）。
18. 待办（可选）：本地 `AgnesGateway-v1.0.0.apk` 与 release-apk/release-apk-v101 目录为旧构建产物，可考虑清理。

19. **v1.0.3 根因修复：上游根地址错误**。用户手机刷新模型列表报 `HTTP 404: <!DOCTYPE html>`。取证发现 `https://agnes-ai.cn` 是官网首页（Next.js），`/v1/models` 与 `/v1/chat/completions` 均返回 HTML 404；真实 API 根地址为 `https://apihub.agnes-ai.cn/v1`（返回标准 OpenAI JSON，无 Key 时 401）。
    - 已改：Go `--upstream` 默认值、Kotlin GatewayService/MainActivity 默认值、XML hint、main_test.go。
    - 新增老配置迁移：`MainActivity.loadPreferences()` 中若读到的值仍是 `https://agnes-ai.cn`，自动替换为新地址。
20. **本地真实端到端验证（关键，非只读代码）**：本地编译 Go 网关 + 真实 Key + **默认参数**（不手动指定 upstream）实测：
    - `GET /v1/models` → 200，返回 9 个真实模型（agnes-2.5-flash / agnes-3.0-flash / agnes-2.5-pro-alpha / agnes-video-2.5 等）。
    - `POST /v1/chat/completions` → 200，正常返回 reasoning_content + content。
    - **多轮工具调用闭环**：第 1 轮 finish_reason=tool_calls 正确带 tool_calls；回传 role:tool 后第 2 轮正确续答（"北京今天晴朗，气温18度，空气质量良好"）。
    - **流式 SSE**：stream=true 逐块 flush chunk 正常。
    - 修前用旧二进制实测确为 404 text/html，形成对照。
21. arm64 二进制重新交叉编译并同步 jniLibs（SHA256 = 1b5af9e6...c51a，两侧一致）；versionCode 4 / versionName 1.0.3。
22. 清理本地测试残留：gateway_test*.exe 及其进程（EPERM 需先 Stop-Process 再删）。

23. **v1.0.4：日志与更新检测**
    - 用户反馈：本地调通后日志框空白。根因：Go 侧只在 429/401/转发失败时打日志，成功请求完全无记录。已新增成功请求日志，格式 `[200] POST /v1/chat/completions model=agnes-2.5-flash | Key sk-x...piLR | 0.77s stream=true`，含状态码/方法/路径/model/脱敏Key/耗时/是否流式。明确不打请求体、响应体、流式分块。
    - 用户问「会不会有性能问题」：已评估，每次一行约 100 字节，20k 字符截断下无压力；真正开销在 TextView 全量重设，已改为增量 append，仅裁剪时重建一次。
    - 用户问「日志何时清除」：原只有 20000 字符滚动截断 + 手动清空 + 进程退出即丢，无时间维度。已改为条数(500) + 时间(30分钟)双限，不持久化（避免 Key 片段落盘）。
    - 日志滚动智能跟随：新增 `autoScrollEnabled` + `isNearBottom()`（48px 容差），用户手动上翻不再被拽回；清空或回到底部恢复。
    - 新增启动时新版本检测 `checkForUpdates()`：拉 GitHub releases/latest，`compareVersion()` 语义化比较，有新版弹 AlertDialog 跳转；6 小时节流；异常静默。
    - 本地实测日志输出三行（models / chat / stream=true）均正确，脱敏 Key 正常。
    - versionCode 5 / versionName 1.0.4；arm64 二进制 SHA256 = 202f71c0...bb34（两侧一致）。

24. **v1.0.5：打开 App 自动启动网关 + 发版跳 4**
    - 新增 `autoStartGatewayIfNeeded()`：onCreate 中调用，若网关未运行且已配置 Key，延迟 300ms 自动拉起；未配置 Key 则写日志「未配置 Key，跳过自动启动」并跳过，避免空号池空转；已运行时不重复启动。
    - **用户新规则：版本号任一位置禁止出现数字 4**，递增需跳过（1.0.3 → 1.0.5）。已写入 `.agents/rules/README.md`（新建规则总入口，含发版流程与上游接口取证）。
    - 因 v1.0.4 已发布且含 4，本次直接升到 **versionCode 6 / versionName 1.0.5**，CHANGELOG 与 README 中的 v1.0.4 引用全部改为 v1.0.5。
25. **Mika 兼容性问题取证（用户反馈）**：Mika（github.com/wangzan101/Mika）配置本网关后报 400 `input: data did not match any variant of untagged enum ResponseInput`。
    - 实测结论：Agnes 上游（LiteLLM 代理）的 `/v1/responses` **不支持 `function_call` / `function_call_output` 类型**。纯文本 `input`（字符串或对象数组）可用；一旦包含工具调用回传即 400。
    - 对照：`/v1/chat/completions` 完全正常，支持多轮工具调用、并行工具调用、`reasoning_content` 回传、SSE 流式。
    - 结论：不是网关问题，是 Mika 使用了 Responses API 协议；需在 Mika 中把 provider 的接口类型选为 **OpenAI Chat Completions** 而非 Responses。

26. **v1.0.6：日志全屏与滚动修复**
    - 日志点击全屏：横屏 Dialog，支持滚动与文本选择复制，全屏下实时刷新。
    - 修复日志无法滚动：原嵌在外层 ScrollView 中触摸被拦截，现 ACTION_DOWN 时 requestDisallowInterceptTouchEvent(true)、ACTION_UP 时释放。
    - 修复日志重复隐患：logStringBuilder 曾被直接赋给 TextView.text 导致共享引用、双重追加，改为传副本。
    - 全屏视图改增量 append，避免每次新日志全量重建 500 行。
    - AndroidManifest 加 configChanges 防旋转重建。
27. **v1.0.7：日志行内局部高亮**
    - colorizeLog 重写为行内 span：时间戳灰色、[xxx] 状态码按段分色加粗、Key 青色加粗；整行基础色逻辑保留。
    - 引入 StyleSpan 加粗。

## 未完成 / 下一步
- 无待办。当前最新版本 v1.0.7。
- 后续发版按 .agents/rules/README.md 流程执行（版本号禁含 4）。
- 真机验证项（用户执行）：卸载旧版后安装 v1.0.3（签名从 debug 切固定 keystore，必须卸载重装）；确认上游地址输入框显示 `https://apihub.agnes-ai.cn/v1`（老版本会自动迁移）；刷新模型列表应出 9 个模型；手机内 Agent 用 `http://127.0.0.1:8080/v1` 调通并验证多轮工具调用。
- 后续发版流程：改版本号 → 更新 CHANGELOG → 提交 push main → `git tag -a vX.Y.Z -m "Release vX.Y.Z" && git push origin vX.Y.Z`。

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
- CI 运行 42062784030（tag v1.0.2）与 42062770819（main）均 success（3m21s / 3m14s）。
- gh（经 http://127.0.0.1:51081 代理）确认 Release v1.0.2 已发布，资产仅 `AgnesGateway-v1.0.2.apk`；Release 列表仍为 v1.0.0 / v1.0.1 / v1.0.2，无被 main 构建覆盖。
- main 构建 artifact 名 `AgnesGateway-APK`（6.1MB，sha256 9663cee1…），head_sha f99c9de，验证硬编码修复生效。
- v1.0.3 本地端到端实测取证：默认参数启动后 `/healthz` 返回 `"upstream":"https://apihub.agnes-ai.cn"`；`/v1/models` 200；工具调用两轮闭环通过；SSE chunk 正常。
- 交叉编译二进制 SHA256：1b5af9e629bc357634348d378a8dd0aaae19f18a6a2766a1e9862bb682bfc51a（core 与 jniLibs 一致）。
- 环境限制：本机无 JDK/Android SDK（java 不在 PATH），无法本地 assembleRelease，APK 由 GitHub Actions 产出。
