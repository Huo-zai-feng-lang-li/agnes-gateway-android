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
28. **v1.0.8：面板与性能重构（四大核心目标落地）**
    - **顶部实时运维面板**：新增号池健康卡片，展示当前使用的 Key（支持截取末尾4位字符如 `...piLR` 或自定义备注名如 `主力号(...piLR)`）、可用 Key 数量、限流 Key 数量、封禁 Key 数量；当所有 Key 均不可用时，系统自动触发 `IMPORTANCE_HIGH` 的浮动横幅/灵动岛样式告警通知并弹窗告警，带状态复位防抖。
    - **性能极速突破 (<1s)**：实现 `FastDNSResolver` 预埋 8 个主流公共 DNS 与活动网络系统 DNS，采用无死锁缓冲通道发起多路并发竞速解析与 10 分钟内存缓存，彻底消除 10s+ 响应延迟；注入系统 `TZ` 时区环境变量并修复本地时间显示。
    - **精简日志与高亮引擎**：去除冗余的历史 Token 与运行时长统计；格式精简为 `[成功/失败/限流/封禁] + 耗时(TTFB) + 模型 + Key/备注`；重构行内局部高亮渲染器。
    - **401 封禁物理剔除与号池自愈**：检测到上游 401 Unauthorized 立即将该 Key 从号池物理移除并重试其他可用 Key；Kotlin 端监听到 401 封禁后自动同步清理输入框中的失效 Key 并更新本地持久化（保留现有备注名）。
    - **并发与架构安全**：经无状态子代理独立审查，使用 `atomic.Pointer[string]` 彻底消除 `lastUsedKey` 的 Data Race，升级 `compareVersion` 稳健过滤非数字后缀，重新编译 ARM64 架构二进制并校验通过。
    - 版本号遵循**跳过数字 4** 规则，版本递增至 **versionCode 9 / versionName 1.0.8**。

29. **v1.0.9：丝滑响应专项（首字延迟 + 切号）**
    - **并发竞速拨号**：`dialIPsFallback` 由串行改为 Happy Eyeballs 并发（IPv4 优先、每 50ms 错峰、首个连通即用并取消其余），单 IP 超时 3s→800ms。消除首 IP 黑洞导致的最坏 12s 串行等待。
    - **全冷却快速失败**：`PickNext` 全冷却时返回 (nil, total)，`forwardRequest` 区分 503 `gateway_pool_cooldown`（带 Retry-After: 30）与 `gateway_pool_empty`，不再白撞一次上游。
    - **明文 Key 泄露修复（实测发现）**：`/_gateway/status` 原本返回完整明文 Key，`KeyState.Key` 的 json tag 改为 `-`。
    - **客户端断开不冷却**：`reqErr` 为 `context.Canceled`/`DeadlineExceeded` 时记 `[中断]` 日志直接返回，避免连环冷却整池（本轮 503 语义会放大该问题）。
    - **DisplayName 竞争修复**：新增 `displayNameOf()` 锁内取快照，冷却/失败/成功三处日志改用它；`MarkCooldown` 锁外读改为锁内取副本。
    - 新增测试 6 个：全冷却返回 nil、空池 (nil,0)、并发拨号绕过黑洞、热更新并发无竞争、status 无明文 Key、客户端断开不冷却。
    - 真实端到端实测（3 个真实 Key）：流式 TTFB 0.34~0.70s；`/_gateway/status` 无明文 Key；Key 轮询均匀。
    - 版本递增至 **versionCode 10 / versionName 1.0.9**。

30. **v1.0.10：修复启动即闪退（阻断级）**
    - 用户反馈 v1.0.9 安装后一打开就闪退。通过 NP Manager MCP 的 `np_device_crash_report` / `np_device_logcat` 取到真实堆栈：
      `java.lang.ExceptionInInitializerError` → `Caused by: PatternSyntaxException: In a character range [x-y], x is greater than y near index 15` → `at com.agnes.gateway.MainActivity.<clinit>(MainActivity.kt:870)`。
    - 根因：`PATTERN_KEY = Regex("""Key:\s*([^\s|->]+)""")` 的字符类里 `-` 位于 `|` 与 `>` 之间，被解析为区间 `\u007C..\u003E`，因 x > y 非法。该正则是 `companion object` 静态常量，类初始化失败导致 Activity 无法实例化。
    - 修复：改为 `Key:\s*([^\s|>-]+)`（`-` 置于字符类末尾）。已用 Node 逐一校验 8 条正则全部合法。
    - 影响范围：v1.0.8 引入该正则，v1.0.8 / v1.0.9 均会闪退；v1.0.7 不受影响。
    - 版本递增至 **versionCode 11 / versionName 1.0.10**。

31. **v1.0.11：极速反代网关性能优化（429限流极速切号 + 流式首字延迟TTFB保障）**
    - **全号池动态重试覆盖**：重试尝试次数 `maxAttempts` 动态自动适配号池 Key 总数（`max(len(keys), maxRetries)`），只要池中还有未冷却的可用 Key，绝不提前返回失败。
    - **Retry-After 智能解析**：精准解析上游 429 响应头中的 `Retry-After`（支持秒数与 HTTP 时间，300s 钳位），避免固化的长冷却。
    - **双 Client 传输层隔离与首字超时切号**：为流式长请求（`stream: true`）配备专属 `streamClient`（12s 响应头超时）。只要上游首字响应超过 12s 立即判定该 Key 假死，毫秒级标记避让并极速切号，根治客户端长等待。
    - **底层连接池自愈清理**：连接发生超时或异常切号时，主动执行 `CloseIdleConnections`，防止亚健康 HTTP/2 损坏连接被下一个 Key 复用污染。
    - **网关启动预热（WarmUp）**：网关启动时自动后台预热上游 `/models` 连接，消除首字冷启动延迟。
    - **精准日志语义与 UI 高亮渲染**：严格区分 `[限流]`、`[超时]`、`[避让]`、`[切号]`、`[中断]`；Android 端日志高亮引擎全面适配。
    - 单元测试全量绿灯通过（新增 4 个针对 429 切号、流式首字超时切号、Retry-After 解析、可用号统计测试）。
    - 版本遵循禁止包含数字 4 规则，递增至 **versionCode 12 / versionName 1.0.11**。

32. **v1.0.12：修复上游 403 被误记成功并透传（客户端 `Invalid API key` / 长任务突然终止）**
    - 现象：客户端长任务突然报 `Invalid API key` 终止，但手机网关日志只有绿色 `[成功]`，看不到错误。
    - 根因：`forwardRequest` 只处理 429/401/5xx，400/403/404/422 全部落入成功分支；403 既不记错误也不冷却换号，原样透传。真实上游实测 13 个 Key 中 `sk-qSp...RaNO` 对 `/v1/models` 与 `agnes-3.0-flash` 均返回 403「unusual activity / temporarily restricted」，长任务轮询到该 Key 即触发。
    - 修复：新增 403 分类（风控 10m / 额度 30m / 模型无权限 30m / 网段 30m / 通用 60s），冷却并立即切号；全部 Key 都 403 时才回传最后一次上游原文。400/404/422 记 `[失败]` 并原样回传，不换号不冷却；5xx/401 日志补上游原文（300 字截断 + `sk-` 脱敏）。
    - 可观测性：新增 `[请求]` 入口日志、`[完成]/[断流]/[中断]` 流式结束日志；Android `PATTERN_TAG` / `colorizeLog` 适配 `[请求]/[完成]/[断流]/[拒绝]/[风控]`。
    - 单测新增 4 个：403 风控切号、全池 403 回传、400 不换号不冷却、403 分类表；`go test ./...` / `go vet ./...` 通过。
    - 真实端到端：风控 Key + 正常 Key 同池连续 4 次请求全部 200，日志依次出现 `[请求]` → `[风控] HTTP 403 ... 冷却 10m0s` → `[切号]` → `[成功] HTTP 200`；仅放风控 Key 时客户端收到 403 原文 + `[告警] 号池全部不可用`，不再出现 `[成功]`。
    - 版本递增至 **versionCode 13 / versionName 1.0.12**；arm64 与 jniLibs SHA256 = `AC2CD7F633E74E9D9523EEE9A120DEE0CB89A1072CEECEFAD9DE50537950613F`（两侧一致）。

33. **v1.0.13：403 封禁剔除 + 日志可观测性 + UI 精简**
    - 口径确认：2.5 / 3.0 Flash 为免费模型，仅有每分钟请求限制，403 即 Key 被封。
    - 403：风控类及免费模型上非额度/权限/网段的 403 不再冷却，直接 `RemoveKey` 物理剔除并记入封禁黑名单；有可用 Key 立即 `[切号]`，全池被封才回传上游 403。客户端沿用 `[封禁] ... 已从号池剔除` 自动清理本地配置。
    - 备注：Go 删除 `KeyState.Alias`，`parseKeyEntry` 只取 Key 并丢弃旧后缀，展示只保留尾 4 位掩码；Android 删除号池备注文案、hint、脚注，旧配置加载时自动剥离备注。
    - UI：删除顶部“封禁”指标卡（内部 `banned_keys` 保留用于全池告警）；日志正文不再点击横屏，只有“全屏”按钮触发；顶部 `tvStatus` 改为运行中呼吸 + 成功缩放/失败抖动动效。
    - 日志：`GatewayService` 增加 500 条/30 分钟环形缓存与 `seq/time` 广播字段，App 回前台补拉并去重；2xx 在 `w.WriteHeader` 后立即记 `[成功]`，流式结束追加 `[完成]`；启动命令 `--keys=***` 脱敏。
    - 测试：新增免费模型通用 403 封禁用例；403 风控用例改为断言 Key 被物理剔除。`go test -count=1 ./...` 与 `go vet ./...` 通过。
    - 版本递增至 **versionCode 14 / versionName 1.0.13**；arm64 与 jniLibs SHA256 = `13AC18D443988C72CCBAE49C7860905D3A46B7CFB2F65855E25FCC60CB964768`（两侧一致）。

34. **v1.0.15：日志分色、顶部水波纹与数据量格式化**
    - `[成功]` 保持鲜绿色，`[完成]` 改为浅蓝色（`#79C0FF`），流式结束不再与成功同色。
    - 新增 `StatusRippleView`：中心圆点 + 两圈扩散水波纹；顶部显示“运行中 + HH:MM:SS”，停止时灰色静态点并隐藏时长；成功/失败波纹短暂变绿/变红。
    - `GatewayService` 新增 `startedAtMs`，前台服务启动时记录，停止时归零。
    - Go 新增 `formatByteSize`，`[完成]/[断流]/[中断]` 的下发字节数改为 B/KB/MB/GB。
    - 新增字节格式化单测；`go test -count=1 ./...` 与 `go vet ./...` 通过。
    - 版本递增至 **versionCode 15 / versionName 1.0.15**。

## 未完成 / 下一步
- v1.0.15 已发布：https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.15
- Release 资产：`AgnesGateway-v1.0.15.apk`（7,168,884 B，sha256 `29C18618F161430A01E14A3EE2236C51D5342CE6292F826E839F95B68951058E`）。
- 待用户在真机验收：顶部水波纹+运行时长、`[成功]/[完成]` 分色、`下发 KB/MB` 显示。

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
- core/agnes-gateway-arm64 与 app/src/main/jniLibs/arm64-v8a/libgateway.so SHA256 一致：3BBA7B85A42ED218A917E3C3E0A976FCB00800E8D6FED62144AEBA3C0C0B1971（v1.0.9 并发拨号版）。
- 二进制与暂存区源码一致性复核：用暂存区 main.go 重编得 05df751d…，与提交的 agnes-gateway-arm64 / libgateway.so 一致（此前 A03F82… 是含未提交加固代码的版本，已重编对齐）。
- 对抗性审计闭环（.agents/v1.0.8面板与性能重构-审计.md）：通过 Happy Eyeballs IPv4 优先、401 封禁全量告警兜底、banned_key_list 黑名单精准比对、后台独立通知监听，裁决为 PASS。
- workflow YAML 校验通过。
- CI 运行 42062784030（tag v1.0.2）与 42062770819（main）均 success（3m21s / 3m14s）。
- gh（经 http://127.0.0.1:51081 代理）确认 Release v1.0.2 已发布，资产仅 `AgnesGateway-v1.0.2.apk`；Release 列表仍为 v1.0.0 / v1.0.1 / v1.0.2，无被 main 构建覆盖。
- main 构建 artifact 名 `AgnesGateway-APK`（6.1MB，sha256 9663cee1…），head_sha f99c9de，验证硬编码修复生效。
- v1.0.3 本地端到端实测取证：默认参数启动后 `/healthz` 返回 `"upstream":"https://apihub.agnes-ai.cn"`；`/v1/models` 200；工具调用两轮闭环通过；SSE chunk 正常。
- 交叉编译二进制 SHA256：1b5af9e629bc357634348d378a8dd0aaae19f18a6a2766a1e9862bb682bfc51a（core 与 jniLibs 一致）。
- 环境限制：本机无 JDK/Android SDK（java 不在 PATH），无法本地 assembleRelease，APK 由 GitHub Actions 产出。
