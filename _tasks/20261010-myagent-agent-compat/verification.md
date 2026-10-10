# cc-connect × myagent 本地部署与实测记录

## 环境

- 日期：2026-10-10
- cc-connect：本仓库改动版（提交 `97bc9bab`），二进制 `/vm/project/github/cc-connect/cc-connect`（`dev` 版本）
- myagent：`/vm/project/gitee/myagent/bin/myagent`（2026-10-10 重新构建，含 ACP 与 `tool_call_update.title` 修复）
- 模型凭据：沿用用户级 `~/.myagent/.auth`

## 部署拓扑（隔离实例，不动现有服务）

现有主实例保持不变：`/usr/bin/cc-connect`（systemd user 服务 `cc-connect`，`~/.cc-connect/config.toml`，项目 9 个）。

本次新增**独立测试实例**：

| 项 | 值 |
| --- | --- |
| systemd 服务 | `cc-connect-myagent.service`（user 级，已 enable） |
| ExecStart | `/vm/project/github/cc-connect/cc-connect --config /home/code/.cc-connect-myagent/config.toml` |
| 配置 | `~/.cc-connect-myagent/config.toml` |
| 数据目录 | `~/.cc-connect-myagent/data`（session / cron / 日志隔离） |
| bridge | `:19110`，路径 `/bridge/ws`（网页聊天走这里） |
| 管理端 | `:19820`（含 Web 聊天页） |
| 项目 | `myagent`：`type = "acp"`，`cmd = /vm/project/gitee/myagent/bin/myagent`，`args = ["acp"]`，`memory_file = "AGENTS.md"`，`work_dir = /home/code/work/myagent-test` |
| 平台 | 占位飞书（仅供配置校验；`app_id is invalid` 告警不影响网页聊天） |

## 验证用例与结果

用与 Web 管理端聊天页相同的 bridge 协议客户端（`register` → `message{project:"myagent"}`）驱动，全部通过：

| # | 场景 | 结果 |
| --- | --- | --- |
| 1 | 普通提问（要求只回 `myagent-ok`） | `REPLY: myagent-ok`（bridge → engine → acp → myagent → 模型 → 回包 全链路通） |
| 2 | `/cron setup` | 回执「✅ cc-connect 指令已写入 AGENTS.md」；`~/work/myagent-test/AGENTS.md` 生成 8953 字节，含 `<!-- cc-connect-instructions -->` |
| 3 | 指令注入是否真的生效 | 追问「你现在能通过哪个命令把文件/图片发回给用户？」→ 回答「`cc-connect send`，加 `--image` / `--file`」，证明 AGENTS.md 被 myagent 读入 |
| 4 | `/new` | 回执「✅ 新会话已创建」 |
| 5 | 重启 cc-connect 服务后再对话（同 session_key） | `REPLY: restart-ok`，会话可用 |
| 6 | 工具审批（请求执行 `echo myagent-permission-ok`） | 先收到「⚠️ 等待权限响应。请回复 允许 / 拒绝 / 允许所有」；回复「允许」后工具执行，客户端收到 `$ echo ... / [exit 0] / myagent-permission-ok`（含 myagent 工具输出回传），最终回答正确 |

## 复现命令

```bash
# 状态 / 日志
systemctl --user status cc-connect-myagent
tail -f ~/.cc-connect-myagent/logs/cc-connect.log

# 网页聊天
# 浏览器打开 http://127.0.0.1:19820 ，用 ~/.cc-connect-myagent/config.toml 里 [management].token 登录，
# 选择项目 myagent 即可对话；工具审批会以消息形式要求回复「允许 / 拒绝」。

# 代码改动后重新部署
cd /vm/project/github/cc-connect && go build -o cc-connect ./cmd/cc-connect
systemctl --user restart cc-connect-myagent

# 停止 / 卸载测试实例
systemctl --user disable --now cc-connect-myagent
```

接入真实 IM（示例：给 myagent 项目绑一个飞书机器人）：

```bash
/vm/project/github/cc-connect/cc-connect feishu setup \
  -config /home/code/.cc-connect-myagent/config.toml -project myagent
systemctl --user restart cc-connect-myagent
```

（也可把该项目的占位 `[[projects.platforms]]` 换成自己的 `weixin` / `wecom` / `feishu` 凭据。）

## 未验证 / 边界

- 未在真实 IM 平台投递（bridge 平台已验证 engine 全链路；飞书/微信投递属 cc-connect 既有能力，需绑定真实机器人后由用户实测）。
- 图片 / 音频输入：myagent 无多模态，预期走「cc-connect 落盘 + 文本附路径」，未单独验证。
- 未做长时间运行观测（内存、日志轮转、cron 实际触发）。
