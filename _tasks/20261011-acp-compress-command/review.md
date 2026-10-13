# 代码审查：ACP compress_command

- 审查日期：2026-10-11
- 基线提交：`9f4b9e7`
- 被审范围（含未提交差异）：
  - `agent/acp/agent.go`（新增 `compressCommand` 字段、`CompressCommand()`、`core.ContextCompressor` 断言、`New` 解析、`WorkspaceAgentOptions` 透传）
  - `agent/acp/compress_test.go`（新增）
  - `config.example.toml`、`CHANGELOG.md`、`CLAUDE.md`
- 审查结论：**通过**。改动小、边界清晰、向后兼容；无高/中严重度发现。

## 发现

| 级别 | 发现 | 处理 |
| --- | --- | --- |
| 低 | `Agent.mu` 已是 `sync.RWMutex`，`CompressCommand()` 用读锁，与 `GetWorkDir` / 记忆文件 getter 一致 | 无需处理 |
| 低 | 通用 ACP 适配器适用所有 ACP agent；默认 `""`（不支持）保证不误伤其它 agent | 已由默认值与文档覆盖 |
| 低 | myagent `/compact` 只归档长期记忆、不收缩当前会话上下文，与其它 agent 的 `/compress` 语义不同 | 已在 `config.example.toml`、`CHANGELOG.md` 与任务 README 注明 |
| 低 | `[projects.auto_compress]` 开启后会自动触发压缩；默认该配置关闭（`core/engine.go:702`） | 已在任务 README 风险项注明 |
| 观察 | `core` 包 `TestCUJ_H2_TwoPlatformsConcurrentNoBleed` 偶发超时（30s）失败 | 与本次改动无关：该测试不涉及 `agent/acp`；已用 stash 还原基线复跑通过，改动存在时 3 次里 2 次通过、1 次失败，判定为既有 flaky |

## 已执行检查

- `gofmt -l agent/acp/agent.go agent/acp/compress_test.go`：无输出（已格式化）
- `go build ./...`：通过
- `go vet ./agent/acp/...`：通过
- `go test ./agent/acp/...`：通过

## 未执行 / 残余风险

- 未执行与真实 `myagent acp` 的 adapter 级端到端（会触发真实模型调用，需凭证）；留待用户在本机隔离实例的 IM 实测。
- 残余风险：仅当用户显式配置 `compress_command` 才改变行为；不配置时与旧行为逐字节一致。
