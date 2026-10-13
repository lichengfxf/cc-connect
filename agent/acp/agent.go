package acp

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chenhg5/cc-connect/core"
)

func init() {
	core.RegisterAgent("acp", New)
}

// Agent runs an ACP (Agent Client Protocol) agent subprocess over stdio JSON-RPC.
type Agent struct {
	workDir      string
	cmd          string
	cliExtraArgs []string // extra args from cmd, prepended before args
	args         []string
	staticEnv    map[string]string
	extraEnv     []string
	sessionEnv   []string
	authMethod   string // optional, e.g. "cursor_login" for Cursor CLI (see authenticate RPC)
	displayName  string // optional, for doctor (default "ACP")

	// memoryFile / globalMemoryFile are optional instruction-file paths for
	// core.MemoryFileProvider (e.g. "AGENTS.md" for myagent, which injects
	// the workspace AGENTS.md into its system prompt). Relative paths resolve
	// against the current work_dir; empty means the agent does not advertise
	// memory-file support, so /memory and /cron setup keep behaving as before.
	memoryFile       string
	globalMemoryFile string

	// compressCommand is the optional native slash command for
	// core.ContextCompressor (e.g. "/compact" for myagent). Empty means the
	// agent does not advertise compression support, so /compress keeps
	// replying "not supported" exactly as before.
	compressCommand string

	// statusCommand is the optional native status slash command for
	// core.StatusCommandProvider (e.g. "/status" for myagent). Empty means the
	// agent does not advertise status support, so /status keeps showing
	// cc-connect's own system status exactly as before.
	statusCommand string

	// mode is the pending permission mode to apply to new sessions.
	// When set, StartSession applies it via session/set_mode right after
	// session/new. Empty means "use whatever the agent selects by default".
	mode string

	// listUnsupported caches a negative result after we probe the agent
	// for sessionCapabilities.list once. Eliminates spawn cost on
	// subsequent `/ls` invocations against agents that don't implement
	// session/list (e.g. some Copilot/OpenClaw builds).
	listUnsupported atomic.Bool

	// modesCache holds the latest `modes` block we observed via
	// session/new or session/load. It's populated by the session
	// handshake so that future PermissionModes() calls can reflect the
	// actual modes this specific ACP agent offers (rather than a
	// hard-coded fallback that may not match).
	modesMu      sync.RWMutex
	modesCache   []core.PermissionModeInfo
	modesCurrent string

	mu sync.RWMutex
}

// sessionCallbacks lets a running acpSession report what it learned
// during the handshake back to its parent Agent. The session is owned
// by cc-connect's engine (not the agent), so without this the agent
// would never see availableModes / capability advertisements.
type sessionCallbacks interface {
	reportModes(block acpModesBlock)
	reportListSupported(supported bool)
}

// Ensure *Agent satisfies sessionCallbacks at compile time.
var _ sessionCallbacks = (*Agent)(nil)

// Ensure *Agent advertises memory-file support to the engine (/memory, /cron setup).
var _ core.MemoryFileProvider = (*Agent)(nil)

// Ensure *Agent advertises context compression when compress_command is set (/compress).
var _ core.ContextCompressor = (*Agent)(nil)

// Ensure *Agent advertises native status forwarding when status_command is set (/status).
var _ core.StatusCommandProvider = (*Agent)(nil)

// New builds an acp agent from project options.
// Required: options["command"] — executable name or path for the ACP agent.
// Optional: options["args"], options["env"], options["auth_method"],
// options["display_name"], options["mode"], options["memory_file"],
// options["global_memory_file"], options["compress_command"],
// options["status_command"].
func New(opts map[string]any) (core.Agent, error) {
	workDir, _ := opts["work_dir"].(string)
	if workDir == "" {
		workDir = "."
	}
	cmdStr, cliExtraArgs := core.ParseCmdOpts(opts, "")
	if cmdStr == "" {
		return nil, fmt.Errorf("acp: agent option \"cmd\" or \"command\" is required (path or name of the ACP agent binary)")
	}
	if _, err := exec.LookPath(cmdStr); err != nil {
		return nil, fmt.Errorf("acp: command %q not found in PATH: %w", cmdStr, err)
	}

	args := parseStringSlice(opts["args"])
	staticEnv := envMapFromOpts(opts)
	extra := envPairsFromOpts(opts)
	authMethod, _ := opts["auth_method"].(string)
	authMethod = strings.TrimSpace(authMethod)
	displayName, _ := opts["display_name"].(string)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "ACP"
	}
	mode, _ := opts["mode"].(string)
	mode = strings.TrimSpace(mode)
	memoryFile := strings.TrimSpace(stringOpt(opts, "memory_file"))
	globalMemoryFile := strings.TrimSpace(stringOpt(opts, "global_memory_file"))
	compressCommand := strings.TrimSpace(stringOpt(opts, "compress_command"))
	statusCommand := strings.TrimSpace(stringOpt(opts, "status_command"))

	return &Agent{
		workDir:          workDir,
		cmd:              cmdStr,
		cliExtraArgs:     cliExtraArgs,
		args:             args,
		staticEnv:        staticEnv,
		extraEnv:         extra,
		authMethod:       authMethod,
		displayName:      displayName,
		memoryFile:       memoryFile,
		globalMemoryFile: globalMemoryFile,
		compressCommand:  compressCommand,
		statusCommand:    statusCommand,
		mode:             mode,
	}, nil
}

// stringOpt reads a string option; non-string values are ignored (treated as
// unset) rather than coerced into a bogus path.
func stringOpt(opts map[string]any, key string) string {
	if s, ok := opts[key].(string); ok {
		return s
	}
	return ""
}

func envMapFromOpts(opts map[string]any) map[string]string {
	raw, ok := opts["env"]
	if !ok || raw == nil {
		return nil
	}
	switch m := raw.(type) {
	case map[string]string:
		out := make(map[string]string, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(m))
		for k, v := range m {
			out[k] = fmt.Sprint(v)
		}
		return out
	default:
		return nil
	}
}

func envPairsFromOpts(opts map[string]any) []string {
	raw, ok := opts["env"]
	if !ok || raw == nil {
		return nil
	}
	switch m := raw.(type) {
	case map[string]string:
		var out []string
		for k, v := range m {
			out = append(out, k+"="+v)
		}
		return out
	case map[string]any:
		var out []string
		for k, v := range m {
			out = append(out, fmt.Sprintf("%s=%v", k, v))
		}
		return out
	default:
		return nil
	}
}

func parseStringSlice(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), x...)
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			switch t := e.(type) {
			case string:
				out = append(out, t)
			default:
				out = append(out, fmt.Sprint(t))
			}
		}
		return out
	default:
		return nil
	}
}

func (a *Agent) Name() string { return "acp" }

func (a *Agent) SetWorkDir(dir string) {
	a.mu.Lock()
	a.workDir = dir
	a.mu.Unlock()
	slog.Info("acp: work_dir changed", "work_dir", dir)
}

func (a *Agent) GetWorkDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workDir
}

// -- MemoryFileProvider --
//
// memory_file / global_memory_file are opt-in: when unset both getters return
// "", which the engine treats exactly like an agent that does not implement
// MemoryFileProvider ("memory file not supported" for /memory and /cron setup).

// ProjectMemoryFile returns the project-level instruction file, resolving
// relative paths against the current work_dir.
func (a *Agent) ProjectMemoryFile() string {
	a.mu.RLock()
	configured := a.memoryFile
	a.mu.RUnlock()
	return a.resolveMemoryPath(configured)
}

// GlobalMemoryFile returns the user-level instruction file, resolved the same
// way as ProjectMemoryFile.
func (a *Agent) GlobalMemoryFile() string {
	a.mu.RLock()
	configured := a.globalMemoryFile
	a.mu.RUnlock()
	return a.resolveMemoryPath(configured)
}

// resolveMemoryPath expands a configured instruction-file path: absolute paths
// are cleaned and used as-is, relative paths are joined onto the current
// work_dir, and an empty configuration stays empty.
func (a *Agent) resolveMemoryPath(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return ""
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured)
	}
	dir := a.GetWorkDir()
	if dir == "" {
		dir = "."
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Join(dir, configured)
}

// -- ContextCompressor --
//
// compress_command is opt-in: when unset CompressCommand returns "", which the
// engine treats exactly like an agent that does not implement ContextCompressor
// ("this agent does not support context compression" for /compress, /compact).
// The command is a native slash command owned by the downstream ACP agent
// (e.g. "/compact" for myagent); it is forwarded verbatim as a session prompt.

// CompressCommand returns the configured native slash command, or "" when the
// agent does not support context compression.
func (a *Agent) CompressCommand() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.compressCommand
}

// -- StatusCommandProvider --
//
// status_command is opt-in: when unset StatusCommand returns "", which the
// engine treats exactly like an agent that does not implement
// StatusCommandProvider (cc-connect shows its own system status for /status).
// The command is a native slash command owned by the downstream ACP agent
// (e.g. "/status" for myagent); it is forwarded verbatim as a session prompt.

// StatusCommand returns the configured native status slash command, or "" when
// the agent does not support status forwarding.
func (a *Agent) StatusCommand() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.statusCommand
}

func (a *Agent) WorkspaceAgentOptions() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	opts := map[string]any{
		"cmd": a.cmd,
	}
	if len(a.args) > 0 {
		opts["args"] = append([]string(nil), a.args...)
	}
	if len(a.staticEnv) > 0 {
		env := make(map[string]string, len(a.staticEnv))
		for k, v := range a.staticEnv {
			env[k] = v
		}
		opts["env"] = env
	}
	if a.authMethod != "" {
		opts["auth_method"] = a.authMethod
	}
	if a.displayName != "" {
		opts["display_name"] = a.displayName
	}
	if a.memoryFile != "" {
		opts["memory_file"] = a.memoryFile
	}
	if a.globalMemoryFile != "" {
		opts["global_memory_file"] = a.globalMemoryFile
	}
	if a.compressCommand != "" {
		opts["compress_command"] = a.compressCommand
	}
	if a.statusCommand != "" {
		opts["status_command"] = a.statusCommand
	}
	return opts
}

func (a *Agent) SetSessionEnv(env []string) {
	a.mu.Lock()
	a.sessionEnv = env
	a.mu.Unlock()
}

func (a *Agent) StartSession(ctx context.Context, sessionID string) (core.AgentSession, error) {
	a.mu.RLock()
	command := a.cmd
	allArgs := append(append([]string{}, a.cliExtraArgs...), a.args...)
	workDir := a.workDir
	authMethod := a.authMethod
	pendingMode := a.mode
	extra := append([]string(nil), a.extraEnv...)
	extra = append(extra, a.sessionEnv...)
	a.mu.RUnlock()

	return newACPSession(ctx, acpSessionConfig{
		command:         command,
		args:            allArgs,
		extraEnv:        extra,
		workDir:         workDir,
		resumeSessionID: sessionID,
		authMethod:      authMethod,
		initialMode:     pendingMode,
		callbacks:       a,
	})
}

func (a *Agent) Stop() error { return nil }

// -- AgentDoctorInfo --

func (a *Agent) CLIBinaryName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return filepath.Base(a.cmd)
}

func (a *Agent) CLIDisplayName() string {
	a.mu.RLock()
	n := a.displayName
	a.mu.RUnlock()
	if n == "" {
		return "ACP"
	}
	return n
}

// -- ModeSwitcher --
//
// cc-connect's engine treats ModeSwitcher as the point of truth for
// both displaying `/mode` options and applying a mode selection. For
// the generic ACP adapter we keep the Key == ACP modeId so downstream
// `session/set_mode` calls don't need any translation.

// SetMode stores a permission mode to apply to future sessions started
// via StartSession. If the caller-provided mode matches a known cached
// mode id (case-insensitive), it is normalised to that id. Otherwise
// it is stored as-is — some IM users may configure modes before the
// agent has started any session and thus advertised its mode list.
func (a *Agent) SetMode(mode string) {
	normalised := mode
	if m := a.matchModeID(mode); m != "" {
		normalised = m
	}
	a.mu.Lock()
	a.mode = normalised
	a.mu.Unlock()
	slog.Info("acp: mode changed for future sessions", "mode", normalised)
}

// GetMode returns the mode cc-connect will treat as "current" when
// rendering the `/mode` picker or applying SetLiveMode.
//
// Precedence: the most recent explicit SetMode wins (that's the user's
// intent — `/mode plan` should immediately be reflected in the next
// `/mode` listing even before the session/set_mode RPC has returned).
// Only if no one has ever called SetMode for this Agent do we fall
// back to whatever the server advertised as currentModeId during the
// last handshake.
func (a *Agent) GetMode() string {
	a.mu.RLock()
	pending := a.mode
	a.mu.RUnlock()
	if pending != "" {
		return pending
	}
	a.modesMu.RLock()
	defer a.modesMu.RUnlock()
	return a.modesCurrent
}

// PermissionModes returns the modes this ACP agent offers. The list is
// populated from the latest `modes.availableModes` observed on
// session/new or session/load; before the first successful handshake
// it returns an empty slice, and the engine will hide the mode picker.
//
// ACP doesn't send per-mode Desc/NameZh, so Description (if the server
// sent one) maps to Desc for both locales. IM-side translators are
// free to map well-known ids to localised strings later.
func (a *Agent) PermissionModes() []core.PermissionModeInfo {
	a.modesMu.RLock()
	defer a.modesMu.RUnlock()
	out := make([]core.PermissionModeInfo, len(a.modesCache))
	copy(out, a.modesCache)
	return out
}

// matchModeID returns the canonical mode id for a user-typed string
// (case-insensitive match on id or display name). Empty string if no
// match or if we haven't observed modes yet.
func (a *Agent) matchModeID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	lower := strings.ToLower(input)
	a.modesMu.RLock()
	defer a.modesMu.RUnlock()
	for _, m := range a.modesCache {
		if strings.ToLower(m.Key) == lower || strings.ToLower(m.Name) == lower {
			return m.Key
		}
	}
	return ""
}

// -- sessionCallbacks impl --

func (a *Agent) reportModes(block acpModesBlock) {
	infos := make([]core.PermissionModeInfo, 0, len(block.AvailableModes))
	for _, m := range block.AvailableModes {
		infos = append(infos, core.PermissionModeInfo{
			Key:    m.ID,
			Name:   m.Name,
			NameZh: m.Name,
			Desc:   m.Description,
			DescZh: m.Description,
		})
	}
	a.modesMu.Lock()
	a.modesCache = infos
	a.modesCurrent = block.CurrentModeID
	a.modesMu.Unlock()
}

func (a *Agent) reportListSupported(supported bool) {
	if !supported {
		a.listUnsupported.Store(true)
	} else {
		a.listUnsupported.Store(false)
	}
}
