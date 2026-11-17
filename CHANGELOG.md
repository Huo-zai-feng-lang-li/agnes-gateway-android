# 更新日志

本项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [v1.0.16] - 2026-11-17

### 优化
- **模型名日志简写**：`agnes-3.0-flash` 显示为 `A3F`，`agnes-2.5-flash` 显示为 `A2.5F`，`agnes-3.0-pro` 显示为 `A3P`；多段后缀取首字母，如 `agnes-3.0-flash-preview` 显示为 `A3FP`。
- 非 Agnes 模型（如 `gpt-4o`）保持原名，空模型继续显示 `default`；日志高亮同步适配简写。

### 验证
- 新增 `shortModelName` 单测，覆盖大小写、2.5 版本、多段后缀、非 Agnes 模型与空值；`go test -count=1 ./...` 与 `go vet ./...` 通过。

## [v1.0.16] - 2026-11-17

### 优化
- **模型名简写**：日志中的 `agnes-3.0-flash` 显示为 `A3F`，`agnes-2.5-flash` 显示为 `A2.5F`，`agnes-3.0-pro` 显示为 `A3P`；非 Agnes 模型保持原名。
- **Key 只留后四位**：去掉 `Key: ...` 前缀与省略号，日志只显示 `piLR`、`IhIM` 这样的尾号；号池面板与当前 Key 同步精简。
- **预估字数**：`[完成]/[断流]/[中断]` 在 `下发 1.8 KB` 后追加 `· 约 624 字`，按 UTF-8 中文 3 字节/字粗略估算。

### 验证
- 新增 `shortModelName`、`estimateCharCount` 单测；`go test -count=1 ./...` 与 `go vet ./...` 通过。
- GitHub Actions：main CI `44110649383` 成功；tag CI `44111310045` 成功并发布 Release。
- Release APK：`AgnesGateway-v1.0.16.apk`，7,170,460 B，SHA256 `345EFB7727AB6A909D510C03CEC543E53EF6087211A589F8B89328A88C910C36`（本地下载核对一致）。
- arm64 内核 SHA256：`7BA45C24AF2AE21B8F76BE8759797E12FAA4A1EAC7130CBB2CC8655208E75CCC`；`core/agnes-gateway-arm64` 与 `jniLibs/arm64-v8a/libgateway.so` 一致。

## [v1.0.15] - 2026-11-17

### 优化
- **`[成功]` 与 `[完成]` 日志分色**：成功仍为鲜绿色，完成改为浅蓝色，流式调用结束不再和成功混成同一条绿线。
- **顶部水波纹状态动效**：状态点改为“中心圆点 + 两圈向外扩散并淡出的水波纹”；运行中显示“运行中 + HH:MM:SS”运行时长，停止时回归灰色静态圆点并隐藏时长；成功/失败时波纹短暂变绿/变红，1.6 秒后恢复。
- **下发数据量人类可读**：`[完成] / [断流] / [中断]` 日志中的 `下发 6844 B` 改为 `下发 6.7 KB`，超过 1024 KB 自动转 MB，超过 1024 MB 自动转 GB。

### 验证
- 新增 `formatByteSize` 单测（0 B、999 B、1.0 KB、6.7 KB、1.0 MB、3.0 MB）；`go test -count=1 ./...` 与 `go vet ./...` 通过。
- GitHub Actions：main CI `44104309143` 成功；tag CI `44105221201` 成功并发布 Release。
- Release APK：`AgnesGateway-v1.0.15.apk`，7,168,884 B，SHA256 `29C18618F161430A01E14A3EE2236C51D5342CE6292F826E839F95B68951058E`（本地下载核对一致）。
- arm64 内核 SHA256：`11BB12039045C65DD0A45BDF5E82D03B04233A86B2992DE402CC7C34CCA687D8`；`core/agnes-gateway-arm64` 与 `jniLibs/arm64-v8a/libgateway.so` 一致。

## [v1.0.13] - 2026-11-17

### 修复与变更
- **403 风控改为直接封禁剔除号池**：按实际口径，2.5 / 3.0 Flash 为免费模型、仅有每分钟限流，风控 403 说明 Key 已封。网关收到风控 403 后不再冷却 10 分钟重试，而是与 401 一样物理剔除该 Key、记入封禁黑名单并立即切号；免费模型上除额度/模型权限/网段外的通用 403 也按封禁处理。客户端检测到 `[封禁] ... 已从号池剔除` 后继续自动清理本地配置。
- **彻底移除 Key 备注功能**：Go 侧删除 `KeyState.Alias`，`parseKeyEntry` 只提取 Key 并丢弃旧配置后缀，日志与状态只显示尾 4 位掩码；Android 侧删除“支持 # / ---- 备注名”等全部文案，旧配置在加载时自动剥离备注。
- **移除顶部“封禁”指标卡**：封禁 Key 已自动从号池删除，不再单独展示；内部 `banned_keys` 计数保留，用于全池封禁告警。
- **日志仅“全屏”按钮触发横屏**：删除日志正文点击监听，避免上下滑动误触旋转；布局中的静态“点击全屏”改为可点击的“全屏”按钮。
- **顶部运行状态动效**：运行中绿色呼吸；每次 `[成功]` 闪 `✓ 调用成功` 缩放动效，失败类日志（失败/拒绝/断流/封禁/风控/限流/超时/告警）抖动显示 `✕ 调用失败`，1.6 秒后自动回到运行中。
- **后台日志不再丢失**：`GatewayService` 增加 500 条 / 30 分钟环形日志缓存，广播附带序号与时间；App 回前台时按序号补拉错过的 `[成功]`、`[封禁]`、`[断流]` 等记录，并按序号去重。
- **成功日志提前到响应头**：所有 2xx 在 `w.WriteHeader` 后立即记 `[成功] HTTP ... | model | Key | stream=...`，流式结束再追加 `[完成]`，非流式不再等正文复制完才记成功；正文断流仍记 `[断流]`，客户端断开仍记 `[中断]`。
- **启动命令脱敏**：日志中的 `--keys=` 统一替换为 `--keys=***`，避免完整 Key 广播到界面。

### 验证
- `go test -count=1 ./...` 通过；新增免费模型通用 403 封禁用例，原有 403 风控切号用例改为断言 Key 被物理剔除。
- `go vet ./...` 通过。
- arm64 交叉编译与 jniLibs SHA256 一致：`13AC18D443988C72CCBAE49C7860905D3A46B7CFB2F65855E25FCC60CB964768`。
- Android 侧 GitHub Actions `main` 构建 44097012997 与 `v1.0.13` 发布构建 44097469886 均 success；Release 资产 `AgnesGateway-v1.0.13.apk`（7,166,056 B，sha256 `417baf850df9a5f407c7089ea64f0a381562955ac229041a613408b200ea0207`）。

## [v1.0.12] - 2026-11-17

### 修复（关键）
- **上游 403 被误记成「成功」并原样透传**：网关此前只处理 429/401/5xx，400/403/404/422 全部落入成功分支，既不打错误日志也不冷却换号。长任务轮询到被上游风控的 Key 时，403 会直接透传给客户端，客户端显示 `Invalid API key` 并终止任务，而手机日志只有绿色 `[成功]`。
- **403 自动冷却与切号**：新增 403 分类（风控限制 / 额度不足 / 模型无权限 / 网段限制 / 通用拒绝），分别冷却 10 分钟、30 分钟、30 分钟、30 分钟、60 秒，并立即切换下一 Key；只有全部 Key 都不可用时才把最后一次上游 403 原文回传客户端。
- **4xx 不再记成功**：400/404/422 等请求级错误记 `[失败]` 并原样回传，不换号、不冷却；5xx 与 401 日志补带上游原文（截断 300 字、`sk-` 凭证脱敏）。
- **长任务断流留痕**：流式读循环新增 `[完成]`（正常结束）、`[断流]`（上游中断或写回客户端失败）、`[中断]`（客户端断开）三类日志，不再静默 `break`。
- **请求入口日志**：每个业务请求新增 `[请求] METHOD PATH | model | stream | from IP`，用于区分「请求没到网关」与「到了但被静默处理」。

### 验证
- 真实上游实测：13 个 Key 中 `sk-qSp...RaNO` 对 `/v1/models` 与 `agnes-3.0-flash` 均返回 403「unusual activity / temporarily restricted」，其余 12 个 Key 正常。
- 修复后同池（风控 Key + 正常 Key）连续 4 次请求全部 200：日志依次出现 `[请求]` → `[风控] HTTP 403 ... 冷却 10m0s` → `[切号]` → `[成功] HTTP 200`。
- 仅放风控 Key 时，客户端收到 403 与上游原文，日志为 `[风控]` + `[告警] 号池全部不可用`，不再出现 `[成功]`。
- 单测覆盖：403 风控切号、全池 403 回传、400 不换号不冷却、403 分类表；`go test ./...` 与 `go vet ./...` 通过。

## [v1.0.11] - 2026-11-09

### 性能与优化
- **限流极速切号与全池重试**：
  - 动态计算重试次数：单次请求的最大尝试次数自动适配号池中现有的 Key 总数，只要号池中仍有健康未冷却的 Key，就绝不因静态重试阈值而提前报错放弃。
  - 动态 `Retry-After` 智能适配：精准解析上游 429 响应头中的 `Retry-After`，按上游指定秒数精准冷却，不再僵化锁定 30 秒。
- **流式首字延迟保障（TTFB）与假死切号**：
  - **双 Client 传输层隔离**：针对流式长请求（`stream: true`）提供独立传输客户端，设定 12s 响应头超时阈值；当检测到某个 Key 首字卡死，毫秒级标记避让并瞬间切换至下一可用 Key，彻底根治客户端漫长等待与超时。
  - **连接池自愈清理**：连接发生超时或异常切号时，主动释放底层空闲连接（`CloseIdleConnections`），避免亚健康 HTTP/2 复用连带污染下一个 Key。
  - **网络链路预热（WarmUp）**：网关启动时自动后台预热上游 DNS 解析与 TLS 握手长连接，消灭首次调用的冷启动延迟。
- **日志语义精确度与高亮引擎强化**：
  - 严格区分 429 限流（`[限流]`）、首字超时（`[超时]`）、网络异常（`[避让]`）、极速切换（`[切号]`）与客户端断开（`[中断]`）。
  - 运维界面日志渲染引擎同步支持 `[超时]`、`[避让]`、`[切号]`、`[中断]` 专属行内高亮与分色。

## [v1.0.10] - 2026-11-08

### 修复
- **启动即闪退（阻断级）**：日志高亮正则 `PATTERN_KEY` 的字符类写成了 `[^\s|->]`，Java 正则引擎把 `|->` 解析为从 `|`(0x7C) 到 `>`(0x3E) 的区间，由于 x > y 直接抛 `PatternSyntaxException`。该正则是 `MainActivity` 的 `companion object` 静态常量，类初始化失败导致 `ExceptionInInitializerError`，App 一打开就崩溃。改为把 `-` 放到字符类末尾 `[^\s|>-]`。

> v1.0.8 / v1.0.9 均受此问题影响（正则自 v1.0.8 引入），v1.0.7 不受影响。

## [v1.0.9] - 2026-11-08

### 性能与优化
- **并发竞速拨号（Happy Eyeballs）**：候选 IP 由串行逐个尝试改为并发竞速——IPv4 优先，首个 IP 立即拨号，后续每 50ms 错峰启动，谁先连通即用谁并取消其余；单 IP 超时由 3s 降至 800ms。彻底消除「首个 IP 是不可达黑洞时串行等待满超时」带来的额外首字延迟（最坏 12s → 约 1s）。
- **号池全冷却快速失败**：所有 Key 均处于冷却时，不再挑一个冷却中的 Key 去撞上游白等一次往返，而是立即返回 `503 gateway_pool_cooldown` 并附 `Retry-After: 30`；空池单独返回 `gateway_pool_empty`，便于客户端区分。

### 修复
- **管理接口明文 Key 泄露**：`/_gateway/status` 此前会返回完整明文 API Key，现统一置为不参与 JSON 序列化，仅保留 `masked_key` / `display_name`。
- **客户端断开误伤号池**：客户端主动断开或超时导致上下文取消时，不再把 Key 打入 30 秒冷却（此前会连环冷却整池），改为记录 `[中断]` 日志后直接返回。
- **日志读取 Key 备注名的数据竞争**：冷却、失败、成功三处日志改为在锁内取展示名快照，消除与热更新号池并发写入的竞争。

## [v1.0.8] - 2026-11-06

### 新增
- **顶部实时运维面板**：
  - 实时展示当前正在使用的 Key（末尾 4 位字符截断显示或自定义备注名如 `主力号(...piLR)`）。
  - 实时指标显示可用 Key、限流冷却 Key、已封禁 Key 数量与号池健康状态。
  - **全不可用强提醒**：当所有 Key 均被限流或封禁时，触发 `IMPORTANCE_HIGH` 系统灵动岛/浮动弹窗 Heads-up 通知与前台告警横幅（带防抖复位）。
- **Key 自定义备注名支持**：
  - 号池输入框支持使用 `#`、`----`、` `、`\t` 等分隔符定义备注（例如 `sk-xxx#主力号` 或 `sk-xxx----备用号`）。
  - 日志与运维面板自动联动显示备注名。
- **401 封禁物理剔除与号池自愈**：
  - 网关检测到上游 401 Unauthorized 凭证失效时，立即从在体号池物理移除该 Key，避免死循环消耗重试次数。
  - Android 客户端同步自动清理保存的配置，剔除失效 Key 并保留现有各 Key 的备注信息。

### 性能与优化
- **并发 DNS 竞速与内存缓存 (<1s 响应)**：
  - 实现 `FastDNSResolver` 预埋 8 个主流公共 DNS 并结合系统当前 DNS 进行并发竞速解析（Race）。
  - 引入 10 分钟 TTL 内存 DNS 缓存层，将域名解析耗时彻底压缩至毫秒级，彻底根治此前 10s+ 的网络阻塞延迟。
  - 注入系统 `TZ` 环境变量并修正日志时区，确保时间显示绝对准确。
- **精简日志与高亮引擎**：
  - 剔除冗余的历史 Token 统计与总运行时长，精炼输出核心指标：`[成功/限流/封禁/失败] + TTFB首字响应时间 + 模型 + Key/备注`。
  - 重构行内正则局部高亮引擎，状态标签分色加粗，视觉更加清爽聚焦。

## [v1.0.7] - 2026-11-06

### 优化
- **日志行内局部高亮**：提升日志阅读效率，突出关键信息
  - **时间戳**：灰色呈现（`2026/11/06 01:45:51.123456`）
  - **状态码**：按 HTTP 状态段分色并**加粗**（2xx 绿 / 4xx-5xx 红 / 其他蓝）
  - **Key**：青色并**加粗**（`Key sk-x...piLR`），快速定位本次调用的账号
  - 保留原有整行按级别分色逻辑

## [v1.0.6] - 2026-11-06

### 新增
- **日志点击全屏查看**：点击日志区域弹出横屏全屏窗口，空间更大、支持滚动与文本选择复制；全屏状态下新日志仍实时刷新并自动跟随。

### 修复
- **日志区域无法手动滚动**：日志框嵌在外层 ScrollView 中，触摸事件被外层拦截。现按下时拦截外层事件、抬手时释放，内层可独立上下滑动。
- **日志重复显示的隐患**：`logStringBuilder` 曾被直接赋给 `TextView.text`，导致 TextView 内部持有同一实例，后续两处各追加一次造成重复。现改为传入副本。

### 优化
- **日志按级别分色**：成功绿 / 错误红 / 系统蓝 / 号池紫 / 分隔线灰 / 普通白，底色调整为 `#0D1117`，字号 12sp，行间距 2dp，对比度与可读性明显提升。
- **全屏视图改增量追加**：原每次新日志都全量重建全屏文本（最多 500 行），现改为 `append`，仅在裁剪时重建一次。

## [v1.0.5] - 2026-11-06

### 新增
- **打开 App 自动启动反代网关**：已配置 Key 的情况下，启动 App 即自动拉起网关，无需再手动点「启动」；未配置 Key 时跳过并写日志提示，避免空号池空转。服务已在运行时不会重复启动。
- **成功请求日志**：此前仅在 429 / 401 / 转发失败时记录，正常调用完全无日志，导致本地调通后日志框空白。现每次成功请求输出一行元信息：
  ```
  [200] POST /v1/chat/completions model=agnes-2.5-flash | Key sk-x...piLR | 0.77s stream=true
  ```
  - 含状态码、方法、路径、model、**脱敏 Key（可判断本次用的是哪个账号）**、耗时、是否流式。
  - 明确不打请求体 / 响应体 / 流式分块内容，避免隐私泄露与性能开销。
- **启动时新版本检测**：读取 GitHub Release 最新 tag，与本地 versionName 比较，有新版时弹窗提示并提供跳转下载；6 小时内最多检查一次，网络失败静默忽略。

### 优化
- **日志滚动智能跟随**：此前每来一行都强制滚到底部，手动上翻会被立刻拽回。现仅当用户停留在底部附近时才自动跟随；手动上翻后不再打断阅读。点「清空日志」或手动回到底部即恢复跟随。
- **日志清理策略改为条数 + 时间双限**：保留最近 500 条且不超过 30 分钟（原为按 20000 字符截断），内存可控且更符合排查习惯。
- **日志渲染改为增量 append**：原每来一行就 `tvLogs.text = 全量字符串`，高频日志下开销大；现仅在裁剪发生时重建一次。

### 验证（本地真实运行）
- `[200] GET /v1/models | Key sk-x...piLR | 0.40s`
- `[200] POST /v1/chat/completions model=agnes-2.5-flash | Key sk-x...piLR | 0.77s`
- `[200] POST /v1/chat/completions model=agnes-2.5-flash | Key sk-x...piLR | 0.43s stream=true`
- `go test ./...` 全绿；arm64 二进制重新交叉编译并同步（SHA256 = 202f71c0...bb34）。

## [v1.0.3] - 2026-11-06

### 修复（关键）
- **上游根地址默认值错误，导致模型列表刷新失败、对话全部 404**：
  - 原默认值 `https://agnes-ai.cn` 是官网首页（Next.js 站点），请求会返回 HTML 404，客户端表现为「服务商的模型列表不可用（HTTP 404: <!DOCTYPE html>...）」。
  - 真实 API 根地址为 `https://apihub.agnes-ai.cn/v1`（`/v1/models`、`/v1/chat/completions` 均返回标准 OpenAI JSON）。
  - Go 内核 `--upstream` 默认值、Kotlin `GatewayService` / `MainActivity` 默认值、界面输入框 hint、单元测试全部同步更新。
  - **新增老配置迁移**：已保存过 `https://agnes-ai.cn` 的设备，启动时自动替换为新地址，无需手动改配置。

### 验证（本地真实端到端，非只读代码）
用真实 Key 在本地跑 Go 网关（默认参数，不手动指定 upstream）逐项实测：
- `GET /v1/models` → **200**，返回 9 个真实模型（agnes-2.5-flash / agnes-3.0-flash / agnes-2.5-pro-alpha 等）。
- `POST /v1/chat/completions` → **200**，正常返回 `reasoning_content` + `content`。
- **多轮工具调用闭环**：第一轮 `finish_reason: tool_calls` 正确返回 `tool_calls`（含 id/name/arguments）；回传 `role: tool` 结果后第二轮正确续答。
- **流式 SSE 透传**：`stream: true` 逐块 flush `data: {...}` chunk 正常。
- `go test ./...` 全绿；arm64 二进制重新交叉编译并与 `jniLibs` 同步（SHA256 一致）。

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

[v1.0.16]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.16
[v1.0.15]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.15
[v1.0.13]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.13
[v1.0.12]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.12
[v1.0.11]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.11
[v1.0.10]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.10
[v1.0.9]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.9
[v1.0.8]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.8
[v1.0.7]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.7
[v1.0.6]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.6
[v1.0.5]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.5
[v1.0.3]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.3
[v1.0.2]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.2
[v1.0.1]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.1
[v1.0.0]: https://github.com/Huo-zai-feng-lang-li/agnes-gateway-android/releases/tag/v1.0.0
