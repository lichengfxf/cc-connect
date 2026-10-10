# cc-connect 兼容 myagent AI 代理

## 基本信息

- 任务 ID：`20261010-myagent-agent-compat`
- 类型：新需求（跨仓库集成）
- 状态：待验证（`memory_file` 已实现并通过单测 + Engine/bridge 实测；本机隔离实例已部署，等用户在自己的 IM / 浏览器实测确认）
- 创建日期：2026-10-10
- 涉及模块：cc-connect 侧 `agent/acp`、`core/engine.go`、`config.example.toml`、`README.md` / `README.zh-CN.md`、`CHANGELOG.md` / `CLAUDE.md`；myagent 侧 `src/internal/acp/`、`src/cmd/myagent/acp.go`（已在 `/vm/project/gitee/myagent/_tasks/20261010-acp-agent-server/` 交付）

## 原始问题

用户希望 cc-connect 兼容 `/vm/project/gitee/myagent` 这个 AI 代理；myagent 源码可修改。最初只要求方案，后续经「创建 myagent 侧实现任务 → 他人实现 → 两轮独立审查 → cc-connect 侧接入落地」推进。

期望行为：在 IM（Feishu / Telegram / Discord 等）里通过 cc-connect 使用 myagent，并具备与其他 agent 一致的能力——会话续接、工具调用审批、过程输出可见。

## 分析结论

- 接入方式采用**路线 A**：myagent 实现 ACP，cc-connect 复用 `agent/acp`，不新增适配器。myagent 已交付 `myagent acp`（stdio 行分隔 JSON-RPC 2.0），复用 `openRuntime` / `runConversationWithHooks` / `memory.Store` / `systemPromptForTurn`。
- cc-connect 的 `/cron setup`、`/bind setup` 会在 agent 实现 `core.MemoryFileProvider` 时把 `AgentSystemPrompt()`（`cc-connect send`、cron/timer、附件等能力说明）写入项目指令文件；此前的 ACP 适配器未实现该接口，因此自定义 ACP agent（含 myagent）拿不到这些说明。
- 原判断「myagent 工作空间无跨进程锁」**已被推翻**：`workspace.Open` 早已实现工作空间级 flock（`<workspace>/.myagent/tmp/lock`，`LOCK_EX|LOCK_NB`），同一 workspace 同时只允许一个可写实例。

## 修改方案

本次落地 cc-connect 侧两项（路线 A 的收尾）：

1. **`agent/acp` 新增 `memory_file` / `global_memory_file` 选项**（实现 `core.MemoryFileProvider`）：
   - 相对路径按当前 `work_dir` 解析，绝对路径原样使用；`SetWorkDir`（多 workspace）后路径跟随。
   - 未配置时两个 getter 返回空串，`/memory`、`/cron setup` 行为与「不支持指令文件」时完全一致（向后兼容）。
   - `WorkspaceAgentOptions()` 保留这两个选项，workspace 派生 agent 不丢配置。
   - 顺带修掉 `core/engine.go` relay-bind 提示对空路径未判空的问题（否则会打印 `.`）。
2. **配置与文档**：`config.example.toml` 的 ACP 段落新增 `memory_file` 说明与 myagent 示例；`README.md` / `README.zh-CN.md` 的 agent 表新增 myagent 行；`CHANGELOG.md` 与 `CLAUDE.md` 变更记录同步。

myagent 场景配置：

```toml
[[projects]]
name = "myagent"

[projects.agent]
type = "acp"

[projects.agent.options]
work_dir = "/path/to/project"
cmd = "myagent"                 # 或 myagent 二进制的绝对路径
args = ["acp"]
memory_file = "AGENTS.md"       # 相对 work_dir；myagent 默认注入该文件
```

详细设计与决策落地见 [详细设计](design.md)，本次代码审查见 [代码审查](review.md)。

## 详细文档

- [详细设计](design.md)
- [代码审查](review.md)
- [部署与实测记录](verification.md)

## 验收标准

「adapter 级」= 用 cc-connect 真实 `agent/acp` 客户端直接驱动 `myagent acp`；「引擎级」= Engine + 真实 IM 平台。

1. ✅ adapter 级已验证：`type = "acp"` + `cmd`/`args` 能建立会话并返回回答。
2. ✅ adapter 级已验证：`run_command` → `tool_call` → `session/request_permission`，允许后命令执行并回传输出（含命令行 + 退出码 + stdout），`/stop`（`CancelTurn`）后会话仍存活。
3. ✅ 引擎级已验证（bridge 平台，等同真实平台路径）：`/new` 可用、重启 cc-connect 后同会话可继续对话、工具审批「允许」后执行并回显输出（详见 [部署与实测记录](verification.md)）。图片附件属 myagent 既有边界（无多模态），未单独验证。
4. ✅ myagent 侧 `gofmt` / `go vet` / `go build` / `go test` 与 `-race` 全通过（四轮审查中独立复跑）。
5. ✅ cc-connect 侧：`memory_file` 有单测覆盖（`agent/acp/memory_file_test.go`），`go build ./...` / `go test ./...` / `go test ./core/ -run TestCUJ` 全通过；并用真实 Engine 跑 `/cron setup` 验证写入。

## 验证计划

- ✅ cc-connect：`gofmt -l`（本次改动文件）、`go vet ./...`、`go build ./...`、`go test ./...`、`go test ./core/ -run TestCUJ`。
- ✅ 单测：`memory_file` 未配置 / 相对 / 绝对 / 跟随 `SetWorkDir` / workspace 选项透传，共 5 个用例。
- ✅ Engine 实测（本次）：配置 `memory_file = "AGENTS.md"` 时 `/cron setup` 写入 `<work_dir>/AGENTS.md`（8953 字节，含 `<!-- cc-connect-instructions -->` 与 cc-connect 能力说明），回执「✅ cc-connect instructions written to AGENTS.md」；未配置时回执「❌ This agent does not support instruction files.」且不写文件。
- ⏳ 用户在真实 IM / 浏览器上的实操确认（本机隔离实例已就绪，见 [部署与实测记录](verification.md)）。

## 当前进展

- 2026-10-10：完成本任务的分析与方案（[详细设计](design.md)：三条路线对比、决策 D1–D5、风险清单）。
- 2026-10-10：myagent 侧按路线 A 实现 `myagent acp` 并交付，四轮审查记录在 `/vm/project/gitee/myagent/_tasks/20261010-acp-agent-server/`（`3bac7e6` → `2df31fe`），11 项发现无高严重度。
- 2026-10-10：cc-connect 侧独立复审两轮，用真实 `agent/acp` 客户端复跑握手、两轮对话、审批放行 + 命令执行 + 输出回传、`/stop`、工具结果标签，全部通过。
- 2026-10-10（本次）：实现 `agent/acp` 的 `memory_file` / `global_memory_file`、修 `core/engine.go` 空路径判空、补配置示例与 README/CHANGELOG/CLAUDE.md，新增 5 个单测，并完成 Engine 级 `/cron setup` 实测。
- 2026-10-10（本次）：本机部署隔离测试实例 `cc-connect-myagent.service`（bridge `:19110`、管理端 `:19820`、项目 `myagent`），用 bridge 客户端跑通 6 个场景：普通提问、`/cron setup` 写入 `AGENTS.md`、指令注入生效（agent 能答出 `cc-connect send --image/--file`）、`/new`、重启后同会话续接、工具审批 + 命令执行 + 输出回显。详见 [部署与实测记录](verification.md)。

## 文档与发布

- README / README.zh-CN 更新：已更新（agent 表新增 `myagent | ✅ Supported (via ACP)`）
- CHANGELOG / CLAUDE.md 变更记录更新：已更新（`memory_file` 新配置项）
- config.example.toml 更新：已更新（ACP 段新增 `memory_file` 说明 + myagent 示例）
- 用户可感知变化：`type = "acp"` 新增可选 `memory_file` / `global_memory_file`，使 `/cron setup`、`/bind setup` 可向自定义 ACP agent 的指令文件写入 cc-connect 能力说明；未配置时行为不变
- 功能边界：`memory_file` 只负责「写」；agent 自身必须读取该文件才生效（myagent 默认注入工作空间 `AGENTS.md`）。ACP 仍不支持 `session/load` / `session/list` / 模式切换 / 多会话 / 图片输入 / 逐字流式
- 对应提交：本任务记录见 git log `20261010-myagent-agent-compat`；代码改动见本次提交

## 风险与待确认项

- 真实 IM 平台的投递未验证（engine 全链路已由 bridge 平台覆盖；飞书/微信投递属既有能力，需绑定真实机器人实测）。
- 图片 / 音频输入未验证：myagent 无多模态，预期走「落盘 + 文本附路径」。
- `memory_file` 仅保证 cc-connect 侧写入；若配置的路径不是该 ACP agent 真正读取的指令文件，说明不会生效（属配置正确性问题，已在 `config.example.toml` 注明）。
- 相对路径未做工作目录越界校验（可写 `../x`）：配置由运维提供，且与其他 agent 的 memory file 语义一致；如需收紧可后续加校验。
- 单会话语义：一个 `myagent acp` 进程只服务一个 session，第二个 `session/new` 返回 `-32600`；cc-connect 的 `/list`、`/switch` 对 myagent 基本不可用。
- 同一 workspace 互斥：myagent 工作空间 flock 生效，同一 workspace 不能同时跑两个 myagent 实例（含交互 / serve / acp）。
- cc-connect 侧本次改动含 `core/engine.go` 一处判空修正，已跑 CUJ；不改变既有 agent 的行为。

## 下一步

- 用户在本机隔离实例上实测：浏览器打开 `http://127.0.0.1:19820`（token 见 `~/.cc-connect-myagent/config.toml`）→ 项目 `myagent` 对话；如需 IM，按 [部署与实测记录](verification.md) 绑定真实机器人。
- 用户确认通过后把状态推进到「已完成」；如要对外发布，再安排一名审查者独立复审 `core/engine.go` 与 `agent/acp` 的改动。
