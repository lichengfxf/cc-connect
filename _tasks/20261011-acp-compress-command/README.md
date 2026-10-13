# ACP 适配器支持 /compress（接入 myagent 手动压缩）

## 基本信息

- 任务 ID：`20261011-acp-compress-command`
- 类型：新需求（跨仓库集成）
- 状态：待验证（实现完成，单测通过；待真实 myagent / IM 实测）
- 创建日期：2026-10-11
- 涉及模块：cc-connect 侧 `agent/acp/agent.go`、`agent/acp/*_test.go`、`config.example.toml`、`README.md` / `README.zh-CN.md`、`CHANGELOG.md` / `CLAUDE.md`
- 关联任务：`_tasks/20261010-myagent-agent-compat`（路线 A：myagent 实现 ACP、cc-connect 复用 `agent/acp`）

## 原始问题

在飞书通过 cc-connect 使用 myagent 时，发送 `/compact`（或 `/compress`）收到「当前 Agent 不支持上下文压缩」。

期望行为：命令能转发到 myagent 的 ACP 命令通道并触发其手动压缩（myagent `myagent acp` 已支持 `/compact`）。

## 分析结论

- `/compact` / `/compress` 是 cc-connect 的**内置命令**，不会作为普通 prompt 直达 agent：`core/engine.go:6487` 把 `compress` / `compact` 归一到内置 `compress`。
- 内置命令执行前先做能力探测：`core/engine.go:10397`（`cmdCompress`）与 `core/engine.go:10456`（`runCompress`）断言 `agent.(ContextCompressor)`，未实现或 `CompressCommand() == ""` 即回 `MsgCompressNotSupported`（`core/i18n.go:2350`，中文文案即「当前 Agent 不支持上下文压缩」）。
- cc-connect 通用 ACP 适配器 `agent/acp/agent.go` **未实现 `core.ContextCompressor`**，因此对 myagent 永远落到「不支持」分支，命令未发出。
- myagent 侧其实支持：`myagent acp` 的 `handleCommand` 处理 `/compact`（`myagent:src/cmd/myagent/acp.go:254`），命令经 `session.Send` 作为 `session/prompt` 文本即可触发；`/help` 也列出该命令。

## 修改方案

在 ACP 适配器上实现可选接口 `core.ContextCompressor`，命令文本由配置提供（通用适配器不硬编码）。

1. `agent/acp/agent.go`
   - 新增字段 `compressCommand string`。
   - `New` 解析 `options["compress_command"]`（`strings.TrimSpace`，沿用 `stringOpt` / `memory_file` 的写法）。
   - 新增 `func (a *Agent) CompressCommand() string { return a.compressCommand }`。
   - 增加编译期断言 `var _ core.ContextCompressor = (*Agent)(nil)`。
   - `WorkspaceAgentOptions()` 透传非空 `compress_command`（与 `memory_file` 一致，保证多 workspace 派生 agent 不丢配置）。
2. 配置与文档
   - `config.example.toml`：ACP `options` 注释块补 `compress_command` 说明；myagent 示例加 `compress_command = "/compact"`。
   - `README.md` / `README.zh-CN.md`：ACP 适配器可选项补充该配置及 myagent 用法。
   - `CHANGELOG.md` / `CLAUDE.md`：记录本次变更。
3. 测试（`agent/acp/`，沿用 `memory_file_test.go` 风格）
   - 配置时 `CompressCommand()` 返回该值；未配置时返回 `""`（等价「不支持」，向后兼容）。
   - `WorkspaceAgentOptions()` 透传；未配置时不出现该键。

### 语义边界（需在文档中写明）

myagent 的 `/compact` 只归档**长期记忆**，不收缩当前会话上下文窗口（窗口由 `recent_turns` 自动裁剪，见 myagent `src/cmd/myagent/acp.go:261`）。与其它 agent 的「压缩当前上下文」语义不同，回复文案会说明压缩了哪些 turn。

## 设计方案（已定稿，2026-10-11 用户全部按建议确认）

1. 配置项名称与形式：采用 `[projects.agent.options] compress_command`，用户显式配置。
   理由：ACP 是通用适配器，不同 agent 压缩命令不同；与既有 `memory_file` 风格一致，不误伤其它 ACP agent。
2. 未配置时默认：`""`（维持「不支持」，零回归）。
   理由：默认开启会改变所有现有 ACP agent 的 `/compress` 行为（有的会把命令当普通文本发给模型），风险高。
3. 是否做 ACP 能力自动探测：本次不做，只做配置项。
   理由：`available_commands_update` 探测需跨 myagent / cc-connect 两个仓库改协议并联调，工期与回归面大；配置项可最小改动解决。

## 验收标准

- 配置 `compress_command = "/compact"` 后，`/compress` 与 `/compact` 均不再回「不支持」，而是转发到 myagent 并收到压缩结果回复。
- 未配置时，行为与现状完全一致（回「不支持」）。
- 多 workspace（`SetWorkDir` 派生）场景配置不丢失。
- cc-connect：`gofmt -l`（改动文件）、`go vet ./...`、`go build ./...`、`go test ./...` 通过。

## 验证计划

- 单测覆盖上述 `agent/acp` 用例（已完成，见 [verification.md](verification.md)）。
- adapter 级：用户在本机隔离实例（飞书 + myagent）发送 `/compress` 确认回执（步骤见 [verification.md](verification.md)）。

## 详细文档

- [代码审查](review.md)
- [验证记录](verification.md)

## 当前进展

- 2026-10-11：完成根因定位与方案设计；用户确认 3 项决策。
- 2026-10-11：实现 `agent/acp` 的 `core.ContextCompressor`（`compress_command` 选项）、补配置 / CHANGELOG / CLAUDE 文档、新增 4 个单测；`gofmt` / `go build` / `go vet` / `go test ./agent/acp/...` 通过（见 [review.md](review.md)）。全量 `go test ./...` 中 `core` 的 `TestCUJ_H2_TwoPlatformsConcurrentNoBleed` 为既有 flaky（与本次无关，见 [verification.md](verification.md)）。

## 文档与发布

- cc-connect `README.md` / `README.zh-CN.md` 更新：**不需要** —— 两个 README 不维护各 agent 的 options 明细，ACP 可选项权威位置是 `config.example.toml`（已更新）。
- 变更记录更新：需要且已完成 —— `CHANGELOG.md`（Unreleased/Added）、`CLAUDE.md` 变更记录。
- 用户可感知变化：配置 `compress_command` 后，IM 的 `/compress`、`/compact` 转发到下游 ACP agent；未配置时行为不变。
- 功能边界：默认不改变其它 ACP agent 行为；myagent 的 `/compact` 只归档长期记忆，不收缩当前会话上下文。
- 对应提交：`49dcb2a2`（恢复 compress_command；原 `220909e` 在主线对齐时被孤立丢弃后按原 diff 还原）

## 风险与待确认项

- 语义差异：myagent `/compact` 不收缩当前会话上下文，与其它 agent 的 `/compress` 语义不同；需在文档说明。
- `[projects.auto_compress]` 开启时，一旦 `CompressCommand` 非空，myagent 也会被自动压缩触发（默认该配置关闭，见 `core/engine.go:702`）；是否预期需确认。

## 下一步

- 在 cc-connect 配置 myagent 项目加 `compress_command = "/compact"`，重启后在飞书发送 `/compress`，确认回执不再是「不支持上下文压缩」；确认后把本任务标记「已完成」。
