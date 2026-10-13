# 验证记录：ACP compress_command

## 环境

- 仓库：`/home/licheng/work/github/cc-connect`
- 基线提交：`9f4b9e7`，工作区含本任务未提交差异
- 平台：linux / Go（本机）

## 测试矩阵

| # | 场景 | 方法 | 结果 |
| --- | --- | --- | --- |
| 1 | 未配置 `compress_command` | `TestCompressCommand_DefaultsToUnsupported` | 通过：`CompressCommand()==""` |
| 2 | 配置 `/compact` | `TestCompressCommand_ConfiguredValue` | 通过 |
| 3 | 前后空白 trim | `TestCompressCommand_TrimsWhitespace` | 通过 |
| 4 | workspace 透传 / 未配置省略 | `TestWorkspaceAgentOptions_CarriesCompressCommand` | 通过 |
| 5 | 编译期接口实现 | `var _ core.ContextCompressor` | 编译通过 |
| 6 | 构建 | `go build ./...` | 通过 |
| 7 | 静态检查 | `go vet ./agent/acp/...` | 通过 |
| 8 | 包测试 | `go test ./agent/acp/...` | 通过 |
| 9 | 全量测试 | `go test ./...` | `core` 包 `TestCUJ_H2_TwoPlatformsConcurrentNoBleed` 偶发超时失败，见下 |

## 既有 flaky 说明

`TestCUJ_H2_TwoPlatformsConcurrentNoBleed`（`core/cuj_test.go:2021`，「some messages did not land」超时）与本次改动无关：

- 该测试不触及 `agent/acp`；本次仅改 ACP 适配器与文档。
- 用 `git stash -u` 还原到基线复跑：通过（0.09s）。
- 带改动复跑 3 次：2 次通过、1 次 30s 超时失败 → 判定为既有 flaky，不在本任务范围。

## 未执行

- 与真实 `myagent acp` 的 adapter 级 / IM 级端到端：会触发真实模型调用，留待用户在本机隔离实例（飞书）实测。

## 手工验收步骤（用户可执行）

1. cc-connect 配置 myagent 项目新增：

   ```toml
   [projects.agent.options]
   compress_command = "/compact"
   ```

2. 重启 cc-connect，在飞书发送 `/compress`（或 `/compact`）。
3. 期望：不再回「当前 Agent 不支持上下文压缩」，而是 myagent 的压缩回执（如「已压缩 N 个 turn…」或「没有可压缩的已完成对话。」）。
