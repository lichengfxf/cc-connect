# cc-connect 兼容 myagent：详细设计

## 目录

- [1. 背景与证据](#1-背景与证据)
- [2. 三条路线对比](#2-三条路线对比)
- [3. 推荐路线 A：myagent 实现 ACP](#3-推荐路线-amyagent-实现-acp)
- [4. ACP 协议面映射](#4-acp-协议面映射)
- [5. myagent 侧改动清单](#5-myagent-侧改动清单)
- [6. cc-connect 侧改动清单](#6-cc-connect-侧改动清单)
- [7. 风险与开放问题](#7-风险与开放问题)
- [8. 决策落地](#8-决策落地)
- [9. 定稿方向](#9-定稿方向)

## 状态更新（2026-10-10）

本文档写于「只出方案」阶段。路线 A 已被采纳，myagent 侧 `myagent acp` 已实现交付并通过四轮审查，因此第 1–2 节的「myagent 现状」描述（只有交互 / 单轮 / serve 三种形态、无跨进程锁、取消走信号）与第 5–6 节的实施清单都属于当时草案、**部分已过时**；最终落地状态与偏差以 [§8](#8-决策落地)、[§9](#9-定稿方向) 和 myagent 任务记录为准。

- myagent 交付与审查记录：`/vm/project/gitee/myagent/_tasks/20261010-acp-agent-server/`（提交 `3bac7e6` → `bef60ba` → `99d6053` → `85c4a41` → `2df31fe`）。
- 决策 D1–D5 的落地结果见 [§8](#8-决策落地)（2026-10-10 拍板）。
- 工作空间 flock 早已存在于 `workspace.Open`，原「记忆并发写」风险作废。
- cc-connect 侧的 `memory_file` 与配置示例**已落地**（`agent/acp`、`config.example.toml`、README / CHANGELOG）；端到端已在 adapter 级（cc-connect `agent/acp` ↔ `myagent acp`）与 Engine 级 `/cron setup` 验证，真实 IM 平台的完整流程待做。详见 [README](README.md) 与 [代码审查](review.md)。

## 1. 背景与证据

cc-connect 侧：

- Agent 契约：`core/interfaces.go:399`（`Agent`）、`core/interfaces.go:409`（`AgentSession`）；可选能力接口 `ToolAuthorizer`、`ProviderSwitcher`、`HistoryProvider`、`MemoryFileProvider`、`SystemPromptSupporter` 等。
- 注册与创建：`core/registry.go`（`RegisterAgent` / `CreateAgent`），每个 agent 在 `init()` 里注册。
- 对外承诺 ACP 是扩展点：`README.md:240`、`README.md:461`。
- ACP 客户端能力（`agent/acp/`）：
  - `session.go` 的 `handshake` 完成 `initialize` → 可选 `authenticate` → `session/load` 或 `session/new`。
  - `session.go` 的 `Send` 发 `session/prompt`，文本通过 `session/update` 的 `agent_message_chunk` 流出（`agent/acp/mapping.go`）。
  - `onServerRequest` 处理 `session/request_permission`，把工具审批转成 `core.EventPermissionRequest` 交给引擎，用户在 IM 回复后用 `pickPermissionOptionID` / `buildPermissionResult` 回填（`agent/acp/mapping.go:358-414`）。
  - `CancelTurn` 发 `session/cancel` 通知。
  - 每个 `StartSession` 独立启动一个 subprocess（`agent/acp/session.go`），因此服务端可以保持单会话实现。
- （已被本次改动修正）ACP 适配器最初**未实现** `MemoryFileProvider`，因此 cc-connect 的 `/cron setup`、`/bind setup` 对 ACP agent 无指令文件可写，`AgentSystemPrompt()`（`core/interfaces.go:84`，含 `cc-connect send`、cron/timer 等能力说明）无法进入 ACP agent 的上下文。现已在 `agent/acp` 落地可选 `memory_file` / `global_memory_file`。

myagent 侧：

- 形态：交互模式、单轮模式（`myagent "<prompt>"`）、`serve` WebSocket 模式；公共启动逻辑 `openRuntime`（`src/cmd/myagent/main.go:94`）。
- 会话核心：`runConversationWithHooks`（`src/cmd/myagent/conversation.go:47`），带 `agentcore.Hooks`（`OnIntermediateReply` / `OnToolUse` / `OnToolRoundLimit`）。
- 审批：`commandApprover func(localtools.RunCommandInput) bool`（`src/cmd/myagent/terminal.go:168`），同步阻塞返回；交互模式为终端确认，serve 模式为白名单（`src/cmd/myagent/serve.go` 的 `newWhitelistApprover`）。
- serve 单会话语义与协议：`docs/设计文档/serve-system-design.md`（`message` / `turn_started` / `tool_use` / `intermediate` / `reply` / `error`，广播给所有连接）。
- 记忆：工作空间 `.myagent/memory/`，会话边界由 `/new` 写入（`StartNewSession` / `BeginTurn` / `FinishTurn`，`src/internal/memory/session.go`）；跨进程只对 `~/.myagent/user.md` 有 flock（`src/internal/memory/user_lock.go`）。
- 项目规约：`instructions` 模块默认读取并注入工作空间 `AGENTS.md`（可配置 `instructions.enabled` / `max_bytes`）。

## 2. 三条路线对比

| 维度 | A. myagent 实现 ACP | B. cc-connect 新增 `agent/myagent` 走 serve WS | C. 双方自造 stdio JSON 协议 |
| --- | --- | --- | --- |
| 需要改哪个仓库 | 主要 myagent；cc-connect 近零 | 主要 cc-connect；myagent 仍需改（审批、会话） | 两侧都改 |
| 协议性质 | 行业标准 ACP，多客户端复用 | myagent 私有 WS 协议 | 私有协议 |
| cc-connect 复用度 | 复用现成 ACP 客户端（审批/流式/load/list 全有） | 新写适配器，缺审批/会话能力 | 新写适配器 |
| 会话模型匹配 | 每会话一个 myagent 进程，天然匹配 | serve 是单会话共享，与多会话冲突 | 取决于协议设计 |
| IM 工具审批 | ACP `session/request_permission` 直接可用 | 需扩展 serve 协议（当前只有白名单） | 需自行设计 |
| 流式过程输出 | `session/update` 通道现成 | serve 已有 `intermediate` / `tool_use` 可映射 | 需自行设计 |
| 工作量 | myagent 中等偏大；cc-connect 小 | cc-connect 中；myagent 中 | 大 |
| 主要风险 | ACP 细节 + 记忆并发写 | 单会话语义；协议非标准 | 协议长期维护成本 |

结论：选 A。它把复杂度放在「实现一次、多客户端受益」的一侧，并且 cc-connect 的 ACP 客户端已经过 10+ agent 的打磨。

## 3. 推荐路线 A：myagent 实现 ACP

整体形态：

```text
cc-connect（每个 cc-connect 会话一个 ACP 进程）
   │  stdin/stdout：按行 JSON-RPC 2.0
   ▼
myagent acp  ← 复用 openRuntime / 记忆 / 工具 / 审批回调
   │
   └─ 工作空间 <work_dir>/.myagent（记忆、日志）+ <work_dir>/AGENTS.md
```

为什么「每会话一个进程」是关键：myagent 单会话实现不需要为了让多会话工作而重构记忆与 history；cc-connect 的 ACP 适配器本来就为每个 `StartSession` 启动独立子进程，两边模型天然对齐。

首期（v1）实现范围：

- 握手：`initialize`、`session/new`、可选 `session/load`。
- 对话：`session/prompt`、`session/update`（`agent_message_chunk` / `tool_call` / `tool_call_update`）、`session/cancel`。
- 审批：`session/request_permission`。
- 明确不做：`session/list`、模式 / 模型切换、多 session 并存、图片输入。

## 4. ACP 协议面映射

方向说明：req = JSON-RPC request（需要 result），notif = notification（无 id），s→c = 服务端（myagent）发起。

| 方法 / 通知 | 方向 | cc-connect 的发/收行为 | myagent 需要做的 |
| --- | --- | --- | --- |
| `initialize` | c→s req | 发 `protocolVersion=1`、`clientCapabilities.fs/terminal=false`、`clientInfo`；读 `agentCapabilities.loadSession` 与 `agentCapabilities.sessionCapabilities.list` | 返回 `protocolVersion`、`agentCapabilities`（是否支持 load / list）、`agentInfo` |
| `authenticate` | c→s req | 仅当配置了 `auth_method` 才发 | 可不实现（返回 `-32601`）；鉴权走工作空间 `.auth` 文件 |
| `session/new` | c→s req | 发 `{cwd, mcpServers:[]}`，期望 `{sessionId, modes?}` | 分配并返回 `sessionId`；初始化该进程的 `history` |
| `session/load` | c→s req | 发 `{sessionId, cwd, mcpServers:[]}`，失败则回落 `session/new` | 若 advertise `loadSession=true`：按 sessionId 恢复历史；否则返回 `-32601` |
| `session/prompt` | c→s req | 发 `{sessionId, prompt:[{type:"text",text:"..."}]}`；文本通过 update 流出；成功后 cc-connect 自己发 `EventResult` | 执行一轮 `runConversationWithHooks`；期间发 `session/update`；返回 `{"stopReason":"end_turn"}` |
| `session/cancel` | c→s notif | 用户取消 / 超时取消时发 | 取消进行中的 turn context，并让审批等待立即返回拒绝 |
| `session/update` | s→c notif | 解析 `agent_message_chunk` → `EventText`；`tool_call` / `tool_call_update` → `EventToolUse` / `EventToolResult` | 推送最终回答文本与工具调用事件 |
| `session/request_permission` | s→c req | 转成 `EventPermissionRequest` 交给引擎，用户在 IM 回复后回 `{outcome:{outcome:"selected",optionId}}` | 审批时发请求并阻塞等待；按响应 allow/deny |
| `session/list` | c→s req | 仅 advertise 后调用；`-32601`/`-32600` 视为不支持并缓存 | v1 不实现 |
| `session/set_mode` | c→s req | `/mode` 切换时发 | v1 不实现 |

审批选项约定：服务端在 `session/request_permission` 的 `options` 里提供 `{optionId, name, kind}`，`kind` 含 `allow` / `reject` 关键字（cc-connect 靠关键字挑选，见 `agent/acp/mapping.go:364-389`）。建议提供 `allow_once` 与 `reject_once` 两项。

## 5. myagent 侧改动清单

新增包 `src/internal/acp/`：

- `protocol.go`：JSON-RPC 2.0 消息类型（request / response / error / notification），ACP 参数与结果结构体。
- `transport.go`：stdin/stdout 按行编解码；读循环独立 goroutine，保证 turn 执行期间仍能处理 `session/cancel` 与权限响应；协议输出只走 stdout，所有日志只走 stderr / 文件。
- `server.go`：请求分发，`initialize` / `session/new` / `session/load` / `session/prompt` / `session/cancel`。
- `permission.go`：把 `commandApprover` 适配成 ACP 审批：先套用现有 `auto_approve` 前缀规则与 `full_auto`，未命中才发 `session/request_permission` 并阻塞等待响应；等待期间收到 cancel 一律按拒绝处理。
- `mapping.go`：工具事件映射（`run_command` → `tool_call`，含 `rawInput`、`title`、`kind`）。

新增子命令 `src/cmd/myagent/acp.go`：

- `myagent acp`（cobra 子命令），复用 `openRuntime` 完成配置 / 工作空间 / 记忆 / skills / instructions 装配。
- 单进程单活动 session：`session/new` 初始化 `history`（可复用 `rt.history`，即启动时的记忆恢复结果）；是否支持多个 sessionId 见决策 D2。
- 每轮 turn 调用 `runConversationWithHooks`，用 `agentcore.Hooks` 把 `OnIntermediateReply` / `OnToolUse` 转成 `session/update` 通知。
- 复用 `systemPromptForTurn` 组装 system prompt（role.md / user.md / AGENTS.md / skills / memory context）。
- 取消：把现有 `interruptController`（信号驱动）扩展为也可由 `session/cancel` 触发。

测试：

- `src/internal/acp/`：协议编解码、握手、审批 allow/deny、cancel 的单元测试（用内存 pipe 驱动 server，无需真实进程）。
- `src/cmd/myagent/`：`myagent acp` 子命令参数与启动 smoke。
- 端到端：本地测试替身跑通一轮 prompt。

## 6. cc-connect 侧改动清单

必需项（仅配置，无需新代码）：

```toml
[[projects]]
name = "myagent-demo"

[projects.agent]
type = "acp"

[projects.agent.options]
work_dir = "/path/to/workspace"
cmd = "/path/to/myagent"      # 或 "myagent"
args = ["acp"]
# auth_method / env 视需要
```

建议项：

1. `config.example.toml` 增加 myagent via ACP 示例。
2. `README.md` / `README.zh-CN.md` 的 agent 列表或 ACP 说明里点名 myagent（可选）。
3. **`memory_file` 选项（已落地）**：`agent/acp` 新增可选 `memory_file` / `global_memory_file`，实现 `core.MemoryFileProvider`。cc-connect 的 `/cron setup`、`/bind setup` 会把 `AgentSystemPrompt()` 写入 `<work_dir>/<memory_file>`（myagent 场景填 `AGENTS.md`），补上 `cc-connect send`、cron/timer 等能力说明。相对路径按 `work_dir` 解析、绝对路径原样；未配置时行为与「不支持指令文件」一致。纯 cc-connect 侧改动，不动 ACP 协议。
4. 可选：`agent/acp` 增加 fake myagent ACP server 的集成测试，防止协议回归。

## 7. 风险与开放问题

- ~~**记忆并发写**~~（已解决）：`workspace.Open` 早已对 `<workspace>/.myagent/tmp/lock` 加 `LOCK_EX|LOCK_NB` 独占锁，三种运行形态统一复用，同一 workspace 同时只允许一个可写实例。剩余语义影响是「多个 IM 会话绑同一项目会互相拒绝」，属 myagent 单会话语义的既定边界。
- ~~**system prompt 注入**~~（已解决）：见第 6 节第 3 点。`memory_file` 落地后，`/cron setup`、`/bind setup` 可把 `cc-connect send` / cron 等能力说明写入 myagent 注入的 `AGENTS.md`。
- ~~**session/load 语义**~~（已按决策 D2 落地）：首期 `loadSession=false`，进程内 history 从空开始（保证客户端 `/new` 清空上下文），长期上下文由 myagent 记忆按轮注入；`/list`、`/switch` 对 myagent 基本不可用。
- **流式**（已接受为边界）：回答整段一次性推送，非逐字流式；工具事件仍提供过程感。
- **图片 / 附件**（待实测）：myagent 无多模态输入路径，预期走「cc-connect 落盘 + 文本附路径」的文本路径，但未做引擎级验证。
- ~~**取消语义**~~（已解决）：`session/cancel` 经 `context.WithCancelCause` 中断模型 HTTP 与工具执行，turn 以 `stopReason=cancelled` 收尾、进程继续存活；取消登记竞态与停机竞态已在审查中发现并修复，附确定性用例。

## 8. 决策落地

2026-10-10 拍板结果如下：

1. **接入方式：采用 A** —— myagent 实现 ACP，cc-connect 复用 `agent/acp`。已落地，cc-connect 侧不需要新适配器；myagent 的实现见 `src/internal/acp/` + `src/cmd/myagent/acp.go`。
2. **跨进程续接：采用 (a)** —— `initialize` advertise `loadSession=false`，进程内 history 从空开始（保证客户端 `/new` 真正清空上下文），长期上下文靠 myagent 记忆按轮注入。`session/load` 留待后续；影响 cc-connect 的 `/list`、`/switch` 与重启后的短期上下文。
3. **工具审批：采用 (a)** —— 转发到 IM。判定顺序为 `full_auto` 放行 → `auto_approve` 前缀放行 → `confirm_by_default=false` 拒绝 → 其余发 `session/request_permission` 阻塞等待；allow / reject / cancelled 均在真实 cc-connect 客户端下验证过。
4. **`memory_file` 选项：已落地** —— `agent/acp` 实现 `core.MemoryFileProvider`，`/cron setup`、`/bind setup` 可写入 myagent 的 `AGENTS.md`；未配置时保持原有「不支持指令文件」行为。
5. **首期范围：最小闭环** —— `session/list`、模式 / 模型切换返回 `-32601`。实现过程中额外补了工具输出回传（`tool_call_update.content`）与 `tool_call.title` 兜底，属超出原设计的增益。

## 9. 定稿方向

上述决策形成的定稿方向（均已执行）：

- 路线 A（myagent 实现 ACP），cc-connect 仅做配置示例 + 可选 `memory_file`。
- 首期限定：`initialize` / `session/new` / `session/prompt` / `session/update` / `session/cancel` / `session/request_permission`。
- 审批走 IM，保留 `auto_approve` 白名单与 `full_auto` 语义。
- 记忆并发写防护：myagent 侧已有工作空间 flock，无需新增。

剩余动作见任务 [README](README.md) 的「下一步」：真实 IM 平台的引擎级验证（`/new`、重启续接、附件、`/cron setup` 回执）。
