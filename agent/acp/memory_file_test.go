package acp

import (
	"path/filepath"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

// memory_file 是可选项：不配置时 acp agent 对 /memory、/cron setup 的行为
// 必须与「不支持内存文件」一致（两个 getter 都返回空串）。
func TestMemoryFile_DefaultsToUnsupported(t *testing.T) {
	a, err := New(map[string]any{"command": "true"})
	if err != nil {
		t.Fatal(err)
	}
	mp, ok := a.(core.MemoryFileProvider)
	if !ok {
		t.Fatal("acp agent 应实现 core.MemoryFileProvider")
	}
	if got := mp.ProjectMemoryFile(); got != "" {
		t.Fatalf("ProjectMemoryFile = %q, want empty when memory_file is unset", got)
	}
	if got := mp.GlobalMemoryFile(); got != "" {
		t.Fatalf("GlobalMemoryFile = %q, want empty when global_memory_file is unset", got)
	}
}

// 典型用法（myagent）：memory_file = "AGENTS.md" 解析到 <work_dir>/AGENTS.md，
// 也就是 /cron setup 会写入 cc-connect 能力说明的位置。
func TestMemoryFile_RelativeResolvesAgainstWorkDir(t *testing.T) {
	workDir := t.TempDir()
	a, err := New(map[string]any{
		"command":     "true",
		"work_dir":    workDir,
		"memory_file": "AGENTS.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	mp := a.(core.MemoryFileProvider)

	want := filepath.Join(workDir, "AGENTS.md")
	if got := mp.ProjectMemoryFile(); got != want {
		t.Fatalf("ProjectMemoryFile = %q, want %q", got, want)
	}
	if base := filepath.Base(mp.ProjectMemoryFile()); base != "AGENTS.md" {
		t.Fatalf("base = %q, want AGENTS.md（/cron setup 的回执用该文件名）", base)
	}
}

// 绝对路径原样使用；嵌套相对路径按 work_dir 拼接。
func TestMemoryFile_PathResolution(t *testing.T) {
	workDir := t.TempDir()
	abs := filepath.Join(t.TempDir(), "docs", "CLAUDE.md")

	a, err := New(map[string]any{
		"command":            "true",
		"work_dir":           workDir,
		"memory_file":        filepath.Join("docs", "CLAUDE.md"),
		"global_memory_file": abs,
	})
	if err != nil {
		t.Fatal(err)
	}
	mp := a.(core.MemoryFileProvider)

	if got, want := mp.ProjectMemoryFile(), filepath.Join(workDir, "docs", "CLAUDE.md"); got != want {
		t.Fatalf("ProjectMemoryFile = %q, want %q", got, want)
	}
	if got := mp.GlobalMemoryFile(); got != filepath.Clean(abs) {
		t.Fatalf("GlobalMemoryFile = %q, want %q", got, abs)
	}
}

// 多 workspace 场景下 SetWorkDir 会被调用，内存文件路径必须跟着走。
func TestMemoryFile_FollowsSetWorkDir(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	a, err := New(map[string]any{
		"command":     "true",
		"work_dir":    first,
		"memory_file": "AGENTS.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	mp := a.(core.MemoryFileProvider)
	if got, want := mp.ProjectMemoryFile(), filepath.Join(first, "AGENTS.md"); got != want {
		t.Fatalf("before SetWorkDir: %q, want %q", got, want)
	}

	a.(*Agent).SetWorkDir(second)
	if got, want := mp.ProjectMemoryFile(), filepath.Join(second, "AGENTS.md"); got != want {
		t.Fatalf("after SetWorkDir: %q, want %q", got, want)
	}
}

// workspace 派生的 agent 必须保留 memory_file，否则多 workspace 下会丢失 /cron setup 支持。
func TestWorkspaceAgentOptions_CarriesMemoryFile(t *testing.T) {
	a, err := New(map[string]any{
		"command":            "true",
		"memory_file":        "AGENTS.md",
		"global_memory_file": "/tmp/myagent-role.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshotter, ok := a.(core.WorkspaceAgentOptionSnapshotter)
	if !ok {
		t.Fatal("agent does not implement WorkspaceAgentOptionSnapshotter")
	}
	opts := snapshotter.WorkspaceAgentOptions()
	if got, _ := opts["memory_file"].(string); got != "AGENTS.md" {
		t.Fatalf("memory_file = %q, want AGENTS.md", got)
	}
	if got, _ := opts["global_memory_file"].(string); got != "/tmp/myagent-role.md" {
		t.Fatalf("global_memory_file = %q, want /tmp/myagent-role.md", got)
	}
}
