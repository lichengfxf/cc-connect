# 代码审查：cc-connect `status_command` 转发

- 审查日期：2026-10-13
- 基线提交：`9f4b9e7a`（main）
- 被审范围（含未提交差异）：
  - `core/interfaces.go`（新增 `StatusCommandProvider`）
  - `agent/acp/agent.go`（`statusCommand` 字段、`StatusCommand()`、`New` 解析、编译期断言、`WorkspaceAgentOptions` 透传）
  - `core/engine.go`（`cmdStatus` 转发入口 `tryForwardStatusCommand`；泛化 `runCompress`/`processCompressEvents` 为 `runForwardedCommand`/`processForwardedCommandEvents`）
  - `core/i18n.go`（`MsgStatusNoSession`、`MsgStatusEmpty`）
  - `agent/acp/status_test.go`（新增）、`core/engine_test.go`（新增 2 个用例）
  - `config.example.toml`、`CHANGELOG.md`、`CLAUDE.md`
- 审查结论：**通过**。改动沿用既有 `compress_command` 模式，向后兼容（未配置时逐字节维持旧行为）；无高 / 中严重度发现。

## 发现

| 级别 | 发现 | 处理 |
| --- | --- | --- |
| 低 | `/status` 转发会占用会话锁（`TryLock`），Agent 侧 status 应为即时命令；myagent 的 `/status` 是本地只读渲染，无模型调用，符合 | 已在任务 README 风险项注明 |
| 低 | `runCompress` 泛化后，unsupported 分支改为显式 `session.Unlock()`；与原先 deferred unlock 语义等价（调用方已 `TryLock`） | 保留堆砌的事件处理逻辑不动，缩小回归面 |
| 低 | `tryForwardStatusCommand` 在解析 Agent 后不实现接口时返回 false，`cmdStatus` 会再次 `commandContext`（单 workspace 下为常量返回） | 仅一次冗余解析，无副作用 |
| 低 | 转发结果走文本发送（`e.send`），卡片平台也显示文本 | 与 `/compress` 转发一致；Agent 状态本为纯文本 |
| 观察 | `TestCUJ_H2_TwoPlatformsConcurrentNoBleed` 为既有 flaky（见 `20261011-acp-compress-command`） | 与本次无关 |

## 已执行检查

- `gofmt -l agent/acp/agent.go core/engine.go core/interfaces.go`：无输出
- `go vet ./core/... ./agent/acp/...`：通过
- `go build ./...`：通过
- `go test ./agent/...`：全部通过
- `go test ./core/`：通过（47s）
- 新增用例：`TestCmdStatus_ForwardsToStatusAgent`（转发命令并展示回执）、`TestCmdStatus_NoStatusCommand_ShowsOwnStatus`（未配置回归）；`agent/acp/status_test.go` 4 个用例

## 未执行 / 残余风险

- 未与真实 `myagent acp` + 真实 IM 做端到端（会触发真实部署），留待用户在隔离实例实测；adapter 级协议行为已在 `20261010-myagent-agent-compat` 验证通过。
- 残余风险：泛化 `runCompress` 的回归由既有 `/compress` 用例（`TestCmdCompress_*`）覆盖并通过。
