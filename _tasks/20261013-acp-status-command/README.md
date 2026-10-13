# cc-connect 新增 `status_command`：IM `/status` 转发到 ACP Agent

## 基本信息

- 任务 ID：`20261013-acp-status-command`
- 类型：新需求（跨仓库集成，cc-connect 侧）
- 状态：待验证
- 创建日期：2026-10-13
- 涉及模块：cc-connect `core/interfaces.go`、`core/engine.go`、`core/i18n.go`、`agent/acp/agent.go`、`config.example.toml`、`CHANGELOG.md`、`CLAUDE.md`
- 关联任务：
  - myagent 侧 `_tasks/20261013-acp-status-command/`（ACP 实现 `/status`）
  - cc-connect `_tasks/20261011-acp-compress-command/`（本任务沿用其 `compress_command` 的转发模式）
  - cc-connect `_tasks/20261010-myagent-agent-compat/`（ACP 接入）

## 原始问题

用户在 IM 里发 `/status`，看到的是 cc-connect 自身状态（平台 / 运行时长 / 会话 / cron / 版本），没有 myagent 的运行状态。

期望行为：为配置了原生 status 命令的 ACP Agent（如 myagent），IM `/status` 能转发到该 Agent 并展示其结果。

## 分析结论

- `/status` 是 cc-connect 内置命令：`builtinCommands`（`core/engine.go:6473`）注册 → `handleCommand` switch（`core/engine.go:6691`）→ `cmdStatus`（`core/engine.go:8718`）。内置命令不会 fall through 给 Agent，因此 ACP Agent 永远收不到。
- 已有先例：`/compress`、`/compact` 通过 `core.ContextCompressor` + `compress_command` 选项把命令作为 `session/prompt` 转发给 Agent（commit `220909e`，见 `20261011-acp-compress-command`）。`/status` 缺同类机制。
- 通用 ACP 适配器 `agent/acp/agent.go` 是配置驱动的，不应硬编码命令名。
- 结论：新增可选接口 `StatusCommandProvider`（`StatusCommand() string`），ACP 适配器通过 `status_command` 选项实现；`cmdStatus` 在 Agent 实现了该接口且命令非空时，把命令转发给 Agent 并展示其结果，否则维持 cc-connect 自身状态（零回归）。

## 修改方案

1. `core/interfaces.go`：新增可选接口
   ```go
   type StatusCommandProvider interface { StatusCommand() string }
   ```
2. `agent/acp/agent.go`：
   - 新增 `statusCommand` 字段，`New` 解析 `options["status_command"]`（`strings.TrimSpace`）。
   - 新增 `StatusCommand()`；编译期断言 `var _ core.StatusCommandProvider = (*Agent)(nil)`。
   - `WorkspaceAgentOptions()` 透传非空 `status_command`。
3. `core/engine.go`：
   - `cmdStatus` 开头尝试转发：`tryForwardStatusCommand` 在 Agent 实现 `StatusCommandProvider` 且命令非空时，走 `runForwardedCommand` 并返回，否则回落到原有 cc-connect 状态路径。
   - 把 `runCompress` / `processCompressEvents` 泛化为 `runForwardedCommand` / `processForwardedCommandEvents`（`emptyMsgKey` 参数区分空结果回退文案），`runCompress` 作为薄包装保持行为不变。
4. `core/i18n.go`：新增 `MsgStatusNoSession`、`MsgStatusEmpty` 多语言文案。
5. 配置与文档：`config.example.toml` 补 `status_command` 说明与 myagent 示例；`CHANGELOG.md`、`CLAUDE.md` 记录。

## 详细文档

- [代码审查](review.md)

## 验收标准

- Agent 实现 `StatusCommandProvider` 且 `status_command` 非空时，IM `/status` 不再显示 cc-connect 自身状态，而是转发命令并把 Agent 回执展示给用户。
- 未配置 / 未实现接口时，`/status` 行为与现状逐字节一致。
- 无活跃会话时回明确提示，不 panic。
- `gofmt -l`（改动文件）无输出、`go vet ./core/... ./agent/acp/...`、`go build ./...`、`go test ./agent/... ./core/` 通过。

## 验证计划

- 单测：`agent/acp/status_test.go`（默认 / 配置 / trim / workspace 透传）；`core/engine_test.go` 的 `TestCmdStatus_ForwardsToStatusAgent`、`TestCmdStatus_NoStatusCommand_ShowsOwnStatus`。
- 回归：`go build ./...`、`go test ./agent/... ./core/`。
- 端到端（待用户）：myagent 项目配置 `status_command = "/status"`，IM 发 `/status` 确认展示 myagent 状态。

## 当前进展

- 2026-10-13：创建任务并完成根因定位与方案设计（沿用 `compress_command` 模式）。
- 2026-10-13：实现接口 / ACP 选项 / engine 转发与 i18n；新增单测；`gofmt` / `go vet` / `go build` / `go test ./agent/... ./core/` 全通过（见 [review.md](review.md)）。

## 文档与发布

- README / README.zh-CN 更新：不需要 —— 两个 README 不维护各 Agent options 明细，权威位置是 `config.example.toml`（已更新）。
- 变更记录更新：已完成（`CHANGELOG.md`、`CLAUDE.md`）。
- 用户可感知变化：配置 `status_command` 后 IM `/status` 转发到 ACP Agent；未配置时行为不变。
- 功能边界：通用 ACP 适配器，命令名由配置提供；默认不改变其它 Agent。
- 对应提交：`d94c97cc`（status_command 转发）

## 风险与待确认项

- 转发 `/status` 会占用会话锁（与 `/compress` 一致）；Agent 侧 status 应为即时命令而非长任务。
- 泛化 `runCompress` 存在回归风险，已由既有 `/compress` 单测覆盖并通过。
- 真实 IM 端到端未在本地复现（需部署隔离实例）。

## 下一步

- 用户在本机隔离实例配置 `status_command = "/status"` 后，IM 发 `/status` 确认展示 myagent 状态，然后把状态推进到「已完成」。
