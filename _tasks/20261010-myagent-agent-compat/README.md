# cc-connect 兼容 myagent AI 代理

## 基本信息

- 任务 ID：`20261010-myagent-agent-compat`
- 类型：新需求（跨仓库集成）
- 状态：方案已确定（推荐路线 A，待用户拍板决策 D1–D5 后进入实现）
- 创建日期：2026-10-10
- 涉及模块：cc-connect 侧 `agent/`（复用 ACP）、`config.example.toml`、`README.md` / `README.zh-CN.md`、`CHANGELOG.md` / `CLAUDE.md`；myagent 侧 `src/internal/acp/`、`src/cmd/myagent/`

## 原始问题

用户希望 cc-connect 兼容 `/vm/project/gitee/myagent` 这个 AI 代理；myagent 源码可修改。本轮只要求方案，暂不实现。

期望行为：在 IM（Feishu / Telegram / Discord 等）里通过 cc-connect 使用 myagent，并具备与其他 agent 一致的能力——会话续接、工具调用审批、过程输出可见。

## 分析结论

- cc-connect 的接入点是 `core.Agent` / `core.AgentSession`（`core/interfaces.go:399`、`core/interfaces.go:409`），通过 `core.RegisterAgent()` 注册（`core/registry.go`）；已有 10+ 个 agent 适配器，并且对外明确承诺「任何 ACP 兼容的 agent 都可接入」（`README.md:240`、`README.md:461`）。
- myagent 现状：Go 编写的 CLI 对话代理，有交互 / 单轮 / `serve` 三种形态。`serve` 是 WebSocket 服务，但为**单会话模型**——所有连接共享一份历史、按到达顺序串行执行、无 session ID / 会话列表、且没有交互式审批（只能白名单放行，见 `/vm/project/gitee/myagent/docs/设计文档/serve-system-design.md`）。
- 工具审批在 myagent 内是同步回调 `commandApprover func(localtools.RunCommandInput) bool`（`src/cmd/myagent/terminal.go:168`），交互模式在终端确认、serve 模式只走白名单——把它接到 IM 审批需要额外协议通道。
- cc-connect 的 ACP 适配器（`agent/acp/`）已经实现了完整的 ACP 客户端：`initialize`、`session/new`、`session/load`、`session/prompt`、`session/update` 流式事件、`session/request_permission` 审批、可选的 `session/list`，并且**每个 cc-connect 会话独立启动一个 subprocess**（`agent/acp/session.go`）。

## 修改方案

推荐**路线 A：myagent 实现 ACP，cc-connect 复用现有 `agent/acp`**。摘要：

- myagent 新增 `myagent acp` 子命令（stdio、按行分隔的 JSON-RPC 2.0），复用现有 `openRuntime`、`runConversationWithHooks`、`memory.Store`、`systemPromptForTurn`。
- 实现 `initialize` / `session/new` / `session/prompt` / `session/cancel` / `session/update` / `session/request_permission` 最小闭环；`session/list`、模式切换等首期不做。
- cc-connect 侧**不需要新增适配器**：配置 `[projects.agent] type = "acp"` + `cmd`/`command` 指向 myagent 二进制、`args = ["acp"]` 即可。
- cc-connect 可选小改动：给 acp 适配器加 `memory_file` 选项，让 `/setup` 能把 cc-connect 能力说明写入工作空间 `AGENTS.md`（myagent 默认注入 AGENTS.md）。

备选路线 B（cc-connect 新增适配器走 myagent `serve` WebSocket）与 C（双方自造 stdio 私有协议）的对比见 [详细设计](design.md)。

## 详细文档

- [详细设计](design.md)

## 验收标准

1. cc-connect 配置 `type = "acp"`、`cmd = "<myagent 二进制>"`、`args = ["acp"]` 后能启动，IM 里发一句话可收到 myagent 的最终回答。
2. 工具调用过程可见：`run_command` 触发时 IM 侧收到工具/审批事件；回复「允许」后命令执行，回复「拒绝」后不执行。
3. 同一会话连续两轮有上下文；cc-connect `/new` 后上下文清空；重启 cc-connect 后同一 cc-connect 会话可续接（具体语义取决于决策 D2）。
4. `go build ./...`、`go test ./...`、`go test ./core/ -run TestCUJ` 通过；新增测试覆盖 ACP 握手 / 审批 / 取消路径。
5. myagent 侧测试通过（`cd src && go test ./...`），新增 ACP 协议单元测试与端到端 smoke。

## 验证计划

- cc-connect：`go build ./...`、`go test ./...`、`go test ./core/ -run TestCUJ`；扩展 `agent/acp` 已有 fake server 测试（`agent/acp/session_mode_test.go`）。
- myagent：`cd src && go test ./...`；手工用 `printf` 灌入 JSON-RPC 走一遍握手 → prompt → 审批 → cancel。
- 端到端：cc-connect + `myagent acp` 跑通「1 轮对话 + 1 次 run_command 审批 + `/new`」。
- 复杂矩阵在实现阶段写入 `verification.md`。

## 当前进展

- 已完成只读调查：cc-connect 的 agent 接口与 ACP 客户端能力；myagent 的运行形态、serve 协议、会话/审批/记忆模型。
- 已产出推荐方案与备选对比（[详细设计](design.md)）。
- 未修改任何业务代码，未提交。

## 文档与发布

- README / README.zh-CN 更新：待确认（落地后需在 agent 列表/示例中加入 myagent + ACP 配置）
- CHANGELOG / CLAUDE.md 变更记录更新：待确认（属于用户可感知的支持范围变化）
- config.example.toml 更新：需要（新增 myagent via ACP 示例）
- 用户可感知变化：待实现并验证后记录
- 功能边界：待决策 D2 / D3 / D5 锁定后记录
- 对应提交：暂无

## 风险与待确认项

- myagent 工作空间记忆的跨进程写安全：目前只有 `user.md` 有 flock 保护（`src/internal/memory/user_lock.go`），工作空间级 `events.jsonl` / `sessions.jsonl` 没有跨进程锁；ACP 的「每会话一个进程」模型下，同一 workspace 多会话并发写可能互相干扰。
- ACP 没有通道注入 cc-connect 的 `AgentSystemPrompt()`；myagent 依赖工作空间 `AGENTS.md` 注入项目规约（`instructions` 模块），因此 `/setup` 与 capabilities 说明需要依赖决策 D4。
- myagent 是单会话模型，ACP / cc-connect 是多会话模型，`session/load` 与 cc-connect `/switch` 的语义映射需要决策 D2 明确。
- myagent 当前不流式输出 token（一次性拿到完整回答），首期只能把最终回答作为单个 `agent_message_chunk` 发出，IM 上表现为「一次性出现」而非逐字流式。

## 下一步

- 等用户对 [详细设计](design.md) 中「待拍板决策」D1–D5 拍板；确认后先做 myagent 侧 ACP 最小闭环，再补 cc-connect 配置示例、文档和测试。
