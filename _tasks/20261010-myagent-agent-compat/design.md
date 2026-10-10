# cc-connect 兼容 myagent：详细设计

## 目录

- [1. 背景与证据](#1-背景与证据)
- [2. 三条路线对比](#2-三条路线对比)
- [3. 推荐路线 A：myagent 实现 ACP](#3-推荐路线-amyagent-实现-acp)
- [4. ACP 协议面映射](#4-acp-协议面映射)
- [5. myagent 侧改动清单](#5-myagent-侧改动清单)
- [6. cc-connect 侧改动清单](#6-cc-connect-侧改动清单)
- [7. 风险与开放问题](#7-风险与开放问题)
- [8. 待拍板决策](#8-待拍板决策)
- [9. 建议默认定稿方向](#9-建议默认定稿方向)

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
- ACP 适配器目前**未实现** `MemoryFileProvider` / `SystemPromptSupporter`，所以 cc-connect `/setup` 对 ACP agent 无 memory 文件可写，`AgentSystemPrompt()`（`core/interfaces.go:84`，含 `cc-connect send`、cron/timer 等能力说明）不会自动进入 ACP agent 的上下文。

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
3. **`memory_file` 选项（推荐做）**：给 `agent/acp` 增加可选 `memory_file`（默认空），实现 `core.MemoryFileProvider`。cc-connect `/setup` 会把 `AgentSystemPrompt()` 写入 `<work_dir>/<memory_file>`（myagent 场景填 `AGENTS.md`），补上 `cc-connect send`、cron/timer 等能力说明。纯 cc-connect 侧改动，不动 ACP 协议。
4. 可选：`agent/acp` 增加 fake myagent ACP server 的集成测试，防止协议回归。

## 7. 风险与开放问题

- **记忆并发写**：myagent 工作空间 `events.jsonl` / `sessions.jsonl` 目前没有跨进程锁。ACP 每会话一个进程，同一 workspace 多会话（或与 `myagent serve` / 交互模式并存）时会并发写。需要加工作空间级 flock，或明确约定同一 workspace 同时只跑一个 myagent 进程；这是实现前必须确认项。
- **system prompt 注入**：见第 6 节第 3 点；不做的话 myagent 在 IM 里不会知道可以使用 `cc-connect send` / cron 等能力。
- **session/load 语义**：myagent 没有「会话 ID 可寻址历史」这一层现成抽象，需要把 ACP sessionId 映射到记忆 session 边界，或首期只做进程内续接并由记忆兜底。
- **流式**：myagent 一次性拿完整回答，首期表现为整段返回，非逐字流式。
- **图片 / 附件**：myagent 无多模态输入路径；cc-connect 会把图片落盘并在文本里附路径，需确认 myagent 是否接受。
- **取消语义**：`session/cancel` 要能中断模型 HTTP 调用与工具执行；myagent 现有中断走信号，ACP 需要可被 RPC 触发。

## 8. 待拍板决策

1. 决策项：用哪种方式接入。
   建议：A —— myagent 实现 ACP，cc-connect 复用 `agent/acp`。
   备选：A / B（cc-connect 新写 serve WS 适配器）/ C（双方自造 stdio 协议）。
   理由：ACP 是 cc-connect 既定扩展点且客户端已成熟；myagent 一次实现可被 Zed 等其它 ACP 客户端复用；B 与 cc-connect 多会话模型冲突且仍要改 myagent 才能审批；C 复用最少、维护成本最高。
   影响：A 把主要工作量放在 myagent（中等偏大），cc-connect 侧近乎零；B/C 会引入长期私有协议维护。
   需要你拍板：是否采用 A。

2. 决策项：首期怎样处理「跨进程续接」。
   建议：首期 advertise `loadSession=false`，靠 myagent 的记忆自动恢复 + cc-connect 新会话；`session/load` 放到后续迭代。
   备选：(a) 首期只做会话内续接（推荐，先跑通闭环）；(b) 首期就实现 `session/load`（把 ACP sessionId 映射到记忆 session 边界）。
   理由：(a) 能最快验证端到端价值，且 myagent 记忆系统本来就会在工作空间维度恢复上下文；(b) 会让首期多一层会话寻址设计。
   影响：影响 cc-connect `/list`、`/switch`、重启续接的体验完整度。
   需要你拍板：选 (a) 还是 (b)。

3. 决策项：工具审批语义。
   建议：ACP `session/request_permission` 转发到 IM；先套用 myagent 现有 `auto_approve` 前缀白名单自动放行，未命中才弹 IM 审批，`full_auto` 全放行。
   备选：(a) 转发 IM 审批（推荐）；(b) 只用白名单静默放行/拒绝（不弹审批，最省事但体验差）。
   理由：(a) 与其它 agent 的 IM 审批体验一致，也让人类能授权白名单外的命令。
   影响：影响安全边界与使用体验；`full_auto` 仍是「任意 shell」高危开关。
   需要你拍板：选 (a) 还是 (b)。

4. 决策项：是否给 cc-connect 的 `agent/acp` 增加 `memory_file`（`AGENTS.md`）选项，让 `/setup` 能注入 cc-connect 能力说明。
   建议：加。
   备选：加 / 不加（改为手工把说明写进工作空间 AGENTS.md）。
   理由：不加的话 myagent 在 IM 里不知道有 `cc-connect send` / cron，附件与定时任务能力会整体缺失。
   影响：约十几行 cc-connect 代码 + 测试；不加则是可感知的功能缺口。
   需要你拍板：是否加。

5. 决策项：首期范围是否包含 `session/list`、模式/模型切换、多会话并存。
   建议：首期都不做，`session/list` 与模式切换返回 `-32601`。
   备选：仅最小闭环（推荐）/ 连 `session/list` 一起做 / 全做。
   理由：先以最小闭环验证价值，避免一次性铺开。
   影响：影响 cc-connect `/list` 与 `/mode` 的可用性，但不影响主对话链路。
   需要你拍板：最小闭环还是包含 `session/list`。

## 9. 建议默认定稿方向

- 路线 A（myagent 实现 ACP），cc-connect 仅做配置示例 + 可选 `memory_file`。
- 首期限定：`initialize` / `session/new` / `session/prompt` / `session/update` / `session/cancel` / `session/request_permission`。
- 审批走 IM，保留 `auto_approve` 白名单与 `full_auto` 语义。
- 记忆并发写防护作为实现前的前置项，在 myagent 侧解决。

若你认可上述建议，可直接回复「全部按建议执行」，我会据此细化并开始改造（先 myagent ACP 最小闭环，再 cc-connect 侧示例、文档与测试）。
