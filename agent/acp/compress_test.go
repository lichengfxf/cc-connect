package acp

import (
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

// compress_command 是可选项：不配置时 acp agent 对 /compress、/compact 的行为
// 必须与「不支持上下文压缩」一致（CompressCommand 返回空串）。
func TestCompressCommand_DefaultsToUnsupported(t *testing.T) {
	a, err := New(map[string]any{"command": "true"})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := a.(core.ContextCompressor)
	if !ok {
		t.Fatal("acp agent 应实现 core.ContextCompressor")
	}
	if got := c.CompressCommand(); got != "" {
		t.Fatalf("CompressCommand = %q, want empty when compress_command is unset", got)
	}
}

// 典型用法（myagent）：compress_command = "/compact" 原样返回，供引擎作为
// session/prompt 文本转发。
func TestCompressCommand_ConfiguredValue(t *testing.T) {
	a, err := New(map[string]any{
		"command":          "true",
		"compress_command": "/compact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.(core.ContextCompressor).CompressCommand(); got != "/compact" {
		t.Fatalf("CompressCommand = %q, want /compact", got)
	}
}

// 前后空白要 trim，避免把 " /compact " 原样发给下游 agent。
func TestCompressCommand_TrimsWhitespace(t *testing.T) {
	a, err := New(map[string]any{
		"command":          "true",
		"compress_command": "  /compact\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.(core.ContextCompressor).CompressCommand(); got != "/compact" {
		t.Fatalf("CompressCommand = %q, want /compact", got)
	}
}

// workspace 派生的 agent 必须保留 compress_command，否则多 workspace 下会丢失
// /compress 支持；未配置时该键不应出现（避免把空命令透传下去）。
func TestWorkspaceAgentOptions_CarriesCompressCommand(t *testing.T) {
	a, err := New(map[string]any{
		"command":          "true",
		"compress_command": "/compact",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshotter, ok := a.(core.WorkspaceAgentOptionSnapshotter)
	if !ok {
		t.Fatal("agent does not implement WorkspaceAgentOptionSnapshotter")
	}
	if got, _ := snapshotter.WorkspaceAgentOptions()["compress_command"].(string); got != "/compact" {
		t.Fatalf("compress_command = %q, want /compact", got)
	}

	bare, err := New(map[string]any{"command": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := bare.(core.WorkspaceAgentOptionSnapshotter).WorkspaceAgentOptions()["compress_command"]; present {
		t.Fatal("compress_command should be omitted when unset")
	}
}
