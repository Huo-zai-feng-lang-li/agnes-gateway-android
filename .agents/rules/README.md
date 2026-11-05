# 项目规则

本目录存放本项目（Agnes Gateway for Android）的长期约定，作为规则总入口。
AI 助手在会话启动时应读取本文件并遵守。

## 发版规则

### 版本号禁止包含数字 4
- 语义化版本号的任一位置都不得出现数字 `4`（如 `1.0.4`、`1.4.0`、`4.0.0` 均禁止）。
- 递增时**直接跳过 4**：`1.0.3` 的下一个版本是 **`1.0.5`**，不是 `1.0.4`。
- 跨十位同理：`1.0.13` → `1.0.15`；`1.3.x` 系列整体跳过，`1.3.x` 之后直接进 `1.5.x`。
- `versionCode` 为单调递增整数，不受此规则限制（仍需保证严格递增以支持覆盖升级）。

### 发版流程
1. 修改 `app/build.gradle.kts` 的 `versionCode`（+1）与 `versionName`（按上一条规则跳号）。
2. 更新 `CHANGELOG.md`，补对应版本段落与底部链接。
3. 同步 `README.md` 中的「当前版本」、tag 示例、APK 文件名。
4. 更新 `.agents/handoff.md`。
5. 运行 `go test ./...` 与 `go vet ./...`。
6. 交叉编译 arm64 二进制并同步到 `app/src/main/jniLibs/arm64-v8a/libgateway.so`（校验 SHA256 一致）。
7. 提交 → `git push origin main` → `git tag -a vX.Y.Z -m "Release vX.Y.Z"` → `git push origin vX.Y.Z`。
8. CI 仅在 tag 推送时创建 Release；`push main` 只产出 Artifact。
9. 发版后核对 Release 资产名与版本号一致。

## 上游接口事实（已取证）

- 真实 API 根地址：`https://apihub.agnes-ai.cn/v1`（`https://agnes-ai.cn` 是官网首页，返回 HTML 404）。
- 上游为 LiteLLM 代理，支持 `/v1/chat/completions` 与 `/v1/responses`。
- **`/v1/responses` 不支持 `function_call` / `function_call_output` 类型**，会返回 400 `data did not match any variant of untagged enum ResponseInput`。需要工具调用能力的客户端必须使用 `/v1/chat/completions`。
- `/v1/chat/completions` 支持：多轮工具调用、并行工具调用、`reasoning_content` 回传、SSE 流式。
