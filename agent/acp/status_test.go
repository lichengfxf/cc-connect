package acp

import (
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

// status_command 是可选项：不配置时 acp agent 对 /status 的行为必须与
// 「不支持原生状态命令」一致（StatusCommand 返回空串）。
func TestStatusCommand_DefaultsToUnsupported(t *testing.T) {
	a, err := New(map[string]any{"command": "true"})
	if err != nil {
		t.Fatal(err)
	}
	sp, ok := a.(core.StatusCommandProvider)
	if !ok {
		t.Fatal("acp agent 应实现 core.StatusCommandProvider")
	}
	if got := sp.StatusCommand(); got != "" {
		t.Fatalf("StatusCommand = %q, want empty when status_command is unset", got)
	}
}

// 典型用法（myagent）：status_command = "/status" 原样返回，供引擎作为
// session/prompt 文本转发。
func TestStatusCommand_ConfiguredValue(t *testing.T) {
	a, err := New(map[string]any{
		"command":        "true",
		"status_command": "/status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.(core.StatusCommandProvider).StatusCommand(); got != "/status" {
		t.Fatalf("StatusCommand = %q, want /status", got)
	}
}

// 前后空白要 trim，避免把 " /status " 原样发给下游 agent。
func TestStatusCommand_TrimsWhitespace(t *testing.T) {
	a, err := New(map[string]any{
		"command":        "true",
		"status_command": "  /status\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.(core.StatusCommandProvider).StatusCommand(); got != "/status" {
		t.Fatalf("StatusCommand = %q, want /status", got)
	}
}

// workspace 派生的 agent 必须保留 status_command，否则多 workspace 下会丢失
// /status 转发；未配置时该键不应出现。
func TestWorkspaceAgentOptions_CarriesStatusCommand(t *testing.T) {
	a, err := New(map[string]any{
		"command":        "true",
		"status_command": "/status",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshotter, ok := a.(core.WorkspaceAgentOptionSnapshotter)
	if !ok {
		t.Fatal("agent does not implement WorkspaceAgentOptionSnapshotter")
	}
	if got, _ := snapshotter.WorkspaceAgentOptions()["status_command"].(string); got != "/status" {
		t.Fatalf("status_command = %q, want /status", got)
	}

	bare, err := New(map[string]any{"command": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := bare.(core.WorkspaceAgentOptionSnapshotter).WorkspaceAgentOptions()["status_command"]; present {
		t.Fatal("status_command should be omitted when unset")
	}
}
