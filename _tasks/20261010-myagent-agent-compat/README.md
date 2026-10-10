# cc-connect 兼容 myagent AI 代理

## 基本信息

- 任务 ID：`20261010-myagent-agent-compat`
- 类型：新需求（跨仓库集成）
- 状态：实现中（myagent 侧 `myagent acp` 已交付并通过四轮审查；cc-connect 侧接入示例 / 文档与可选的 `memory_file` 未落地）
- 创建日期：2026-10-10
- 涉及模块：cc-connect 侧 `agent/acp`（复用，无需新适配器）、`config.example.toml`、`README.md` / `README.zh-CN.md`、`CHANGELOG.md` / `CLAUDE.md`；myagent 侧 `src/internal/acp/`、`src/cmd/myagent/acp.go`（已在 `/vm/project/gitee/myagent/_tasks/20261010-acp-agent-server/` 交付）

## 原始问题

用户希望 cc-connect 兼容 `/vm/project/gitee/myagent` 这个 AI 代理；myagent 源码可修改。最初只要求方案，后续经「创建 myagent 侧实现任务 → 他人实现 → 两轮独立审查」推进，myagent 侧 ACP 服务端已经落地。

期望行为：在 IM（Feishu / Telegram / Discord 等）里通过 cc-connect 使用 myagent，并具备与其他 agent 一致的能力——会话续接、工具调用审批、过程输出可见。

## 分析结论

- cc-connect 的接入点是 `core.Agent` / `core.AgentSession`（`core/interfaces.go:399`、`core/interfaces.go:409`）；对外承诺「任何 ACP 兼容的 agent 都可接入」（`README.md:240`、`README.md:461`），ACP 客户端（`agent/acp/`）已实现 `initialize` / `session/new` / `session/load` / `session/prompt` / `session/update` 流式事件 / `session/request_permission` 审批 / 可选 `session/list`，且每个 cc-connect 会话独立启动一个 subprocess。
- **决策已落地为路线 A**：myagent 实现 ACP，cc-connect 复用现有 `agent/acp`，不新增适配器。myagent 已新增 `myagent acp`（stdio 行分隔 JSON-RPC 2.0），复用 `openRuntime` / `runConversationWithHooks` / `memory.Store` / `systemPromptForTurn`。
- 实现与本文档原始设计的偏差（记录在 myagent 任务的 design.md §12）：Handler 改为 `Session` 句柄、新增 `Hooks.OnToolResult`、中间回复与最终回答都走 `agent_message_chunk`、工具输出随 `tool_call_update.content` 回传并带 `title`。
- 原判断「myagent 工作空间无跨进程锁」**已被推翻**：`workspace.Open` 早已实现工作空间级 flock（`<workspace>/.myagent/tmp/lock`，`LOCK_EX|LOCK_NB`），ACP 直接复用，同一 workspace 同时只允许一个可写实例。

## 修改方案

路线 A（已采用）：

```toml
[[projects]]
name = "myagent-demo"

[projects.agent]
type = "acp"

[projects.agent.options]
work_dir = "/path/to/workspace"
cmd = "/path/to/myagent"      # 或 PATH 中的 myagent
args = ["acp"]
```

cc-connect 侧剩余工作（均非阻塞）：

- `config.example.toml` 增加 myagent via ACP 示例。
- 可选：给 `agent/acp` 增加 `memory_file`（如 `AGENTS.md`）选项并实现 `core.MemoryFileProvider`，让 cc-connect `/setup` 能把 `AgentSystemPrompt()`（`cc-connect send`、cron/timer 等能力说明）写入工作空间 `AGENTS.md`——myagent 默认注入该文件，否则 IM 里 myagent 不知道这些能力。
- 可选：`README.md` / `README.zh-CN.md` 点名 myagent 属于「已实测的 ACP agent」。

详细方案与备选路线对比见 [详细设计](design.md)（其 §8 已更新为决策落地记录）。

## 详细文档

- [详细设计](design.md)

## 验收标准

下列各项中，「adapter 级」指用 cc-connect 真实 `agent/acp` 客户端直接驱动 `myagent acp`（四轮审查中已完成），「引擎级」指经 cc-connect Engine + IM 平台跑真实用户流程（尚未做）。

1. ✅ adapter 级已验证：`type = "acp"` + `cmd = "<myagent 二进制>"` + `args = ["acp"]` 能建立会话并返回回答。
2. ✅ adapter 级已验证：`run_command` 触发 `tool_call` → `session/request_permission`，回复允许后命令执行并回传输出（`EventToolResult` 带命令行 + 退出码 + stdout），`/stop`（`CancelTurn`）后会话仍存活。
3. ⏳ 部分验证：同一会话连续两轮有上下文（adapter 级已验）；cc-connect `/new` 清空上下文、重启 cc-connect 后同一会话续接属引擎级行为，待引擎级验证。按决策 D2，`loadSession=false` + 进程内 history 从空开始，`/new` 会真正清空短期上下文，重启只能靠 myagent 记忆注入长期上下文。
4. ✅ myagent 侧 `gofmt -l .` / `go vet ./...` / `go build ./...` / `go test ./...` 与 `-race` 均通过（审查者独立重跑）。
5. ⏳ cc-connect 侧：本任务目前无 cc-connect 业务代码改动，故无新增测试；若落地 `memory_file` 选项，需补对应单测并跑 `go build ./...` / `go test ./...`。

## 验证计划

- ✅ myagent：全量 gofmt / vet / build / test、`go test -race`、真实二进制冒烟、官方 ACP v1 schema 逐帧校验（实现者自述）。
- ✅ 端到端（adapter 级）：cc-connect `agent/acp` ↔ `myagent acp`，覆盖握手、两轮对话、审批放行 + 命令执行 + 输出回传、`/stop`、工具结果标签可读。
- ⏳ 引擎级：cc-connect Engine + IM 跑 `/new`、重启续接、图片附件（myagent 无多模态，预期按文本路径附文件路径，需实测）。
- ⏳ cc-connect 侧一旦改动 `agent/acp`：`go build ./...`、`go test ./...`、`go test ./core/ -run TestCUJ`。

## 当前进展

- 2026-10-10：完成本任务的分析与方案，产出 [详细设计](design.md)（三条路线对比、决策 D1–D5、风险清单）。
- 2026-10-10：myagent 侧按路线 A 实现 `myagent acp` 并交付，任务记录与四轮审查在 `/vm/project/gitee/myagent/_tasks/20261010-acp-agent-server/`（提交 `3bac7e6` → `bef60ba` → `99d6053` → `85c4a41` → `2df31fe`）。四轮审查累计 11 项发现，无高严重度；取消 / 停机竞态、工具输出回传、`tool_call.title` 等均已修复。
- 2026-10-10：审查者（cc-connect 侧）用自建 cc-connect `agent/acp` 客户端脚本独立复跑两轮，确认上述修复在真实客户端下生效；最近一轮复跑同时验证了工具结果标签由 `toolu_1` 变为命令行。
- cc-connect 侧**尚未**改动任何业务代码：仍无 myagent 配置示例，`agent/acp` 也还没有 `memory_file` 选项。

## 文档与发布

- README / README.zh-CN 更新：待确认（若决定对外点名 myagent，需要更新 agent 列表 / ACP 说明）
- CHANGELOG / CLAUDE.md 变更记录更新：待确认（取决于 cc-connect 侧是否改动代码；仅加配置示例与文档时按仓库惯例决定）
- config.example.toml 更新：需要（新增 myagent via ACP 示例）
- 用户可感知变化：cc-connect 侧暂无变化（ACP 早已支持任意 ACP agent，myagent 是新的可用组合）；myagent 侧新增 `myagent acp` 运行形态
- 功能边界：myagent ACP v1 不支持 `session/load`、`session/list`、模式切换、多会话、图片 / 音频输入、逐字流式；单进程只服务一个 session
- 对应提交：本任务记录 `7d20a366`（本次状态更新见后续提交）；cc-connect 业务代码无提交

## 风险与待确认项

- `memory_file` 未做：myagent 在 IM 里不知道 `cc-connect send` / cron 等能力，涉及附件与定时任务的能力会缺失；这是 cc-connect 侧唯一的实质功能缺口（可选，需拍板）。
- 单会话语义：一个 `myagent acp` 进程只服务一个 session，第二个 `session/new` 返回 `-32600`；cc-connect 的 `/list`、`/switch` 对 myagent 基本不可用（未 advertise `session/list`、`loadSession=false`）。
- 同一 workspace 互斥：工作空间 flock 已在 myagent 侧生效，同一 workspace 不能同时跑两个 myagent 实例（含交互 / serve / acp）。多 IM 会话绑定同一项目时会互相拒绝，需要 cc-connect 侧按项目拆分或接受该限制。
- 引擎级流程未验证：`/new`、重启续接、图片附件、`/cron` 与 `cc-connect send` 等只在有 `memory_file` 后才谈得上。
- myagent 无逐字流式：回答整段一次性推送，IM 表现为一次性出现（工具事件仍提供过程感）。
- 工作空间记忆跨进程写安全：已由 `workspace.Open` 的 flock 覆盖，不再是风险（原判断已修正）。

## 下一步

- 拍板是否落地 cc-connect 侧可选项：`agent/acp` 的 `memory_file` 选项（推荐，补齐 `/setup` 能力注入）。
- 落地 `config.example.toml` 的 myagent via ACP 示例，并决定是否在 README / README.zh-CN 点名 myagent。
- 做一次引擎级验证（cc-connect 起真实 IM 平台 + myagent acp），覆盖 `/new`、重启续接与附件路径。
