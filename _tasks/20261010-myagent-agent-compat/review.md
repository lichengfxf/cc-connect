# cc-connect 兼容 myagent：代码审查

## 审查信息

- 审查日期：2026-10-10
- 基线提交：`16f26de5`（任务记录同步 myagent ACP 落地状态）
- 被审范围：`16f26de5` 之后的全部改动（含提交时的未提交差异）——`agent/acp/agent.go`、`agent/acp/memory_file_test.go`、`core/engine.go`、`config.example.toml`、`README.md`、`README.zh-CN.md`、`CHANGELOG.md`、`CLAUDE.md`
- 审查方式：逐文件阅读 + 全量 `gofmt` / `go vet` / `go build` / `go test` + `go test ./core/ -run TestCUJ` + 真实 Engine 级 `/cron setup` 冒烟
- 审查者：本任务作者（自审）。**建议在对外发布前另找一名审查者做独立复审**，尤其是 `core/engine.go` 的改动。

## 变更清单

| 文件 | 性质 |
| --- | --- |
| `agent/acp/agent.go` | 新增可选的 `memory_file` / `global_memory_file` 选项与 `core.MemoryFileProvider` 实现；`WorkspaceAgentOptions()` 透传；`gofmt` 对齐 3 行既有字段 |
| `agent/acp/memory_file_test.go` | 新增：5 个用例（未配置 / 相对 / 绝对 / 跟随 `SetWorkDir` / workspace 选项透传） |
| `core/engine.go` | relay-bind 提示对空 memory 路径判空（避免打印 `.`） |
| `config.example.toml` | ACP 段新增 `memory_file` 通用说明与 myagent 示例 |
| `README.md` / `README.zh-CN.md` | agent 表新增 myagent（via ACP） |
| `CHANGELOG.md` / `CLAUDE.md` | 变更记录 |

## 审查结论

**通过（自审），无高严重度问题。** 改动范围小、向后兼容（未配置 `memory_file` 时 `/memory` 与 `/cron setup` 行为与改动前一致），新增能力有单测与 Engine 级冒烟覆盖。

## 发现（按严重级别）

### 低

1. **相对路径未做工作目录越界校验。** `resolveMemoryPath` 对 `../foo` 这类相对路径会照常拼在 `work_dir` 之外。配置由运维提供，且与既有 agent 的 memory file 语义一致（都是「配置即信任」），故不阻塞；如要收紧可在解析后校验 `filepath.Rel(workDir, path)` 不以 `..` 开头。
2. **`memory_file` 只保证 cc-connect 侧写入，不保证 agent 读取。** 若配置的路径并非该 ACP agent 真正加载的指令文件，说明不会生效。已在 `config.example.toml` 明确写出「myagent 默认注入工作空间 AGENTS.md」，属配置正确性风险。
3. **`global_memory_file` 是通用能力，myagent 没有对应用户级指令文件。** 该选项对 myagent 可不填；命名与既有 `MemoryFileProvider` 契约一致，不改。
4. **测试沿用 `command: "true"`。** 与 `agent/acp` 既有测试一致，Unix 可用；若 CI 覆盖 Windows 需换可移植的桩命令（既有测试的同一问题，非本次引入）。

### 信息

5. `core/engine.go` 的判空修正顺带覆盖了「agent 实现 `MemoryFileProvider` 但返回空路径」这一此前未处理的分支（例如 `agent/pi` 在无 HOME 时 `GlobalMemoryFile()` 返回空）；其余调用点（`/memory`、`setupMemoryFile`）本就判空。

## 已执行检查

- `gofmt -l`（本次改动文件）→ 无输出；`go vet ./...`、`go build ./...` 通过。
- `go test ./...` 通过；`go test ./core/ -run TestCUJ` 通过（本次改了 `core/engine.go`，按仓库清单执行）。
- 新增单测 `agent/acp/memory_file_test.go` 5 个用例全通过（含 `SetWorkDir` 跟随与 workspace 选项透传）。
- 真实 Engine 冒烟（临时程序，验证后已移出仓库）：
  - 配置 `memory_file = "AGENTS.md"`：`/cron setup` 回执 `✅ cc-connect instructions written to AGENTS.md`，`<work_dir>/AGENTS.md` 生成 8953 字节，含 `<!-- cc-connect-instructions -->` 与 `cc-connect send` / cron 说明。
  - 未配置：回执 `❌ This agent does not support instruction files.`，且未创建文件（向后兼容）。

## 未执行验证与残余风险

- 未在真实 IM 平台做引擎级端到端（`/new`、重启续接、图片附件、真实 `/cron setup` 回执）。
- 未在多 workspace 模式下实跑一次 workspace 克隆流程（仅单测覆盖 `WorkspaceAgentOptions` 透传；该接口在 `agent/claudecode` / `agent/pi` 等已被大量使用，机制本身有既有测试）。
- 未做 Windows 验证。

## 建议（非阻塞）

- 对外发布前补一次独立复审（重点是 `core/engine.go` 的判空改动与 `agent/acp` 的接口实现）。
- 可选：为 `memory_file` 增加工作目录越界校验。
