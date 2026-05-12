# Audit System Plan

## Goal

Build a global, queryable, exportable audit system for cc-connect that covers:

- all projects
- all platforms
- all users
- all sessions

The system should support:

- traceability: who said what, when, in which session
- operations review: which privileged commands were executed or blocked
- compliance export: export by time range, project, platform, user, or event type
- low coupling: reuse existing `core/` abstractions and avoid hardcoding platform or agent knowledge

## Scope

Phase 1 implements an audit log system only. It does not replace the current
session persistence mechanism.

Included in Phase 1:

- user input events
- agent output events
- command executed / blocked events
- permission requested / approved / denied events
- attachment sent events
- session lifecycle events:
  - created
  - switched
  - deleted
  - rotated
  - compressed

Excluded from Phase 1:

- tamper-proof signing
- external database backends
- full-text search engine
- advanced Web UI audit pages in Phase 1

## Current State

The repository already has two related but separate capabilities:

1. Session persistence
   - `SessionManager` stores per-project session state and message history as JSON.
   - Existing files under `data_dir/sessions/*.json` are snapshots, not append-only audit logs.

2. Partial runtime audit logging
   - `core/engine.go` already emits some `slog.Info("audit: ...")` entries for command execution and blocking.
   - `doctor user-isolation` writes isolation audit reports under `~/.cc-connect/audits/`, but that is host isolation auditing, not conversation or operator auditing.

This plan adds a dedicated structured audit subsystem without overloading the
existing session snapshot files.

## Design Principles

- `core/` remains platform-agnostic and agent-agnostic.
- Audit data is append-only.
- Structured events are preferred over parsing free-form logs.
- Existing session snapshot storage remains unchanged.
- Sensitive values must be redacted before persistence.
- Phase 1 should work with only local filesystem storage.

## Proposed Architecture

Add a new audit subsystem in `core/`:

- `core/audit.go`
  - audit event model
  - event type constants
  - `Auditor` interface
  - config and filter structs

- `core/audit_file.go`
  - default filesystem-backed auditor
  - JSONL append writer
  - daily file rotation

- `core/audit_export.go`
  - read audit files
  - filter by project, user, platform, type, and time range
  - export helpers for JSONL and CSV

- `cmd/cc-connect/audit.go`
  - `cc-connect audit show`
  - `cc-connect audit export`
  - optional `cc-connect audit list`

The `Engine` records structured events through the `Auditor` interface. The
default implementation writes JSONL records to disk.

## Event Model

Use an append-only event record:

```go
type AuditEvent struct {
    ID             string         `json:"id"`
    Timestamp      time.Time      `json:"timestamp"`
    Project        string         `json:"project"`
    EventType      string         `json:"event_type"`
    SessionKey     string         `json:"session_key,omitempty"`
    SessionID      string         `json:"session_id,omitempty"`
    AgentSessionID string         `json:"agent_session_id,omitempty"`

    Platform       string         `json:"platform,omitempty"`
    UserID         string         `json:"user_id,omitempty"`
    UserName       string         `json:"user_name,omitempty"`
    ChatID         string         `json:"chat_id,omitempty"`
    ChatName       string         `json:"chat_name,omitempty"`

    Command        string         `json:"command,omitempty"`
    Result         string         `json:"result,omitempty"`
    Reason         string         `json:"reason,omitempty"`

    Role           string         `json:"role,omitempty"`
    Content        string         `json:"content,omitempty"`
    ContentSHA256  string         `json:"content_sha256,omitempty"`
    Truncated      bool           `json:"truncated,omitempty"`

    Metadata       map[string]any `json:"metadata,omitempty"`
}
```

Recommended event types:

- `message.user`
- `message.agent`
- `command.executed`
- `command.blocked`
- `permission.requested`
- `permission.approved`
- `permission.denied`
- `attachment.sent`
- `session.created`
- `session.switched`
- `session.deleted`
- `session.rotated`
- `session.compressed`

## Storage Format

Phase 1 uses local JSONL files.

Directory:

- default: `~/.cc-connect/audit/`
- configurable via `[audit].dir`

Recommended filename format:

- `audit-YYYY-MM-DD.jsonl`

Each line is a single `AuditEvent` encoded as JSON.

Reasons for JSONL:

- easy append semantics
- shell-friendly for `jq`, `rg`, `awk`, and `sed`
- straightforward export pipeline
- decoupled from current `sessions/*.json` snapshot files

The current session files continue to serve session-state persistence only.

## Configuration

Add a new config block:

```toml
[audit]
enabled = true
dir = ""
format = "jsonl"
include_content = true
max_content_bytes = 8192
hash_content = true
include_agent_output = true
include_user_input = true
include_commands = true
include_permissions = true
include_attachments = true
redact_secrets = true
```

Optional per-project override:

```toml
[[projects]]
name = "foo"

[projects.audit]
enabled = true
include_agent_output = true
```

Recommended defaults:

- audit enabled by default
- content included but truncated
- content hashing enabled
- secret redaction enabled

## Integration Points

Integrate at the `core/engine.go` layer so audit behavior is centralized and
independent from individual platform packages.

### 1. User Input

Record `message.user` before forwarding a normal user message to the agent.

### 2. Agent Output

Record `message.agent` when the final response text is sent to the platform.

Important:

- do not record every streaming fragment by default
- record the final aggregated response for signal over noise

### 3. Commands

Replace or supplement existing `slog.Info("audit: ...")` command log points with
structured auditor events:

- `command.executed`
- `command.blocked`

### 4. Permission Flow

Record:

- `permission.requested`
- `permission.approved`
- `permission.denied`

### 5. Session Lifecycle

Record lifecycle events at:

- `/new`
- `/switch`
- delete flow
- idle rotation
- compression success

### 6. Attachments

Record `attachment.sent` when cc-connect sends files or images back through a platform.

Suggested metadata:

- filename
- size
- MIME type
- path hash or safe relative identifier

Avoid storing raw absolute file paths by default.

## Sensitive Data Handling

This is a hard requirement. The audit log must not become a secret dump.

Minimum protections:

- reuse and extend `core.RedactToken()`
- add a more general `RedactSecrets(string) string`
- redact message content before persistence
- redact command arguments and provider-related sensitive fields
- truncate persisted content

Recommended content policy:

- `content`: redacted and truncated text
- `content_sha256`: hash of original text
- `truncated=true`: whether stored content was shortened

For highly sensitive environments:

- allow `include_content = false`
- still persist metadata, length, and hash

## CLI Plan

Add a top-level `cc-connect audit` command.

Initial subcommands:

- `cc-connect audit show --last 100`
- `cc-connect audit export --from 2026-05-01 --to 2026-05-12 --project foo --format jsonl`
- `cc-connect audit export --format csv`

Optional:

- `cc-connect audit list`

Phase 1 priority is `show` and `export`.

## Web Audit UI Plan

The audit system should include a basic Web UI in Phase 2 so operators can
inspect audit trails without shell access to the host.

This UI should follow the existing management dashboard approach:

- no separate frontend deployment
- served from the existing embedded Web admin UI
- backed by Management API endpoints

### UI Goals

- browse recent audit events across all projects
- filter quickly by project, platform, user, event type, and time range
- open an event detail drawer or panel for full context
- export filtered results without requiring CLI access

### Information Architecture

Add a new top-level audit section in the Web admin UI:

- `Audit`

Recommended subviews:

- `Event Stream`
  - default view
  - chronological list or table of audit events
- `Event Detail`
  - side panel, drawer, or dedicated detail page
  - full event payload, redacted content, metadata, and identifiers
- `Export`
  - lightweight export action in the filter bar or page header

Phase 2 does not need dashboards, charts, or analytics widgets. Start with a
high-signal operational review UI.

### Event Stream View

Recommended columns:

- time
- project
- platform
- user
- chat or group
- event type
- command
- result
- session identifier
- short content preview

Behavior:

- newest first by default
- server-side pagination
- sticky filter bar
- click row to open detail view

Visual emphasis:

- distinguish `command.blocked`, `permission.denied`, and failures with warning styling
- keep normal chat events visually quieter than privileged or denied events

### Filters

Phase 2 filter set:

- project
- platform
- user ID
- user name
- chat or group name
- event type
- command
- result
- session key
- session ID
- time range
- free-text contains search over stored redacted content preview

Defaults:

- time range defaults to recent window, such as last 24 hours
- page size defaults to a conservative server-side value, such as 50

### Event Detail View

The detail view should show:

- timestamp
- project
- platform
- user ID and user name
- chat ID and chat name
- session key
- session ID
- agent session ID
- event type
- command, result, and reason if present
- stored content preview
- truncation flag
- content hash
- structured metadata

If content storage is disabled, the UI should clearly show:

- content omitted by policy
- hash retained if available

### Export UX

Provide export from the current filter state:

- export filtered JSONL
- export filtered CSV

Behavior:

- export should reuse server-side filtering logic
- exported data should match current filter constraints exactly
- export should not require re-entering filters in a separate modal if avoidable

### Permissions and Access Control

Audit data is sensitive. The Web UI must not expose it to regular chat users.

Phase 2 expectation:

- audit endpoints are only exposed through the authenticated management server
- access is limited to users who already have admin access to the management UI

Future extension:

- add more granular RBAC if the management UI later introduces scoped operator roles

### Performance Expectations

For Phase 2:

- use server-side pagination, filtering, and sorting
- do not load full audit files into the browser
- avoid returning full content payloads in the list view unless needed
- return compact previews in list results and full payloads in detail fetches

### Frontend/API Split

Keep the Web UI thin:

- frontend handles filter state, pagination state, rendering, and export triggers
- backend handles file scanning, filtering, pagination, and CSV/JSONL generation

This keeps audit logic centralized and makes CLI and Web behavior consistent.

## Management API Follow-Up

Not part of Phase 1 implementation, but the design should keep this path open:

- `GET /api/v1/audit/events`

Recommended companion endpoints for the Web UI:

- `GET /api/v1/audit/events`
- `GET /api/v1/audit/events/{id}`
- `GET /api/v1/audit/export`

Suggested filters:

- `project`
- `user`
- `platform`
- `from`
- `to`
- `type`
- `limit`
- `offset`
- `command`
- `result`
- `session_key`
- `session_id`
- `q`

CLI and future Web UI should reuse the same core audit reader/filter logic.

Suggested response shape for `GET /api/v1/audit/events`:

```json
{
  "items": [],
  "total": 0,
  "limit": 50,
  "offset": 0
}
```

Suggested response shape for `GET /api/v1/audit/events/{id}`:

```json
{
  "event": {}
}
```

Suggested behavior for `GET /api/v1/audit/export`:

- accepts the same filters as the list endpoint
- requires `format=jsonl` or `format=csv`
- streams the export response

## Implementation Steps

1. Add audit event types, config, and `Auditor` interface in `core/`.
2. Implement a filesystem-backed JSONL auditor.
3. Wire the auditor into `Engine` and record core event types.
4. Add config parsing and sensible defaults.
5. Implement `cc-connect audit show/export`.
6. Add unit and regression tests.
7. Optionally add Management API endpoints afterward.

## Testing Plan

All new functionality should include tests.

### Core audit writer tests

- append behavior is correct
- concurrent writes are safe
- daily rotation works
- invalid/missing directory handling is clear

### Engine integration tests

- normal messages produce `message.user`
- final agent responses produce `message.agent`
- command execution and command blocking produce the right events
- permission flows produce complete audit trails
- session lifecycle events are emitted where expected

### CLI tests

- `audit show` reads recent events correctly
- `audit export` filters by project, user, platform, type, and time range
- JSONL and CSV output formats are correct

### Redaction tests

- API keys and tokens are masked
- sensitive headers are masked
- content truncation and hashing behave correctly

## Risks and Tradeoffs

Main risks:

- recording streaming fragments would create high-volume, low-signal logs
- full content retention increases privacy and disk usage risk
- user/chat metadata completeness differs across platforms

Recommended tradeoffs:

- record only final agent outputs by default
- enable content truncation by default
- treat `project + session_key + timestamp` as the minimum useful audit tuple
- use filesystem JSONL first; delay SQLite or external backends until needed

## Delivery Phases

### Phase 1

- structured audit events
- JSONL file persistence
- CLI `show` and `export`
- core event coverage

### Phase 2

- Management API
- basic Web audit view
- server-side pagination and filtering for audit events
- event detail view
- filtered JSONL and CSV export from the Web UI
- richer filtering and export options

### Phase 3

- tamper-evidence or signing chain
- optional external storage backend
- fine-grained RBAC around audit access

## Expected File Changes

Likely files to add:

- `core/audit.go`
- `core/audit_file.go`
- `core/audit_export.go`
- `core/audit_file_test.go`
- `core/audit_export_test.go`
- `cmd/cc-connect/audit.go`
- `cmd/cc-connect/audit_test.go`
- `docs/usage.md`
- `docs/usage.zh-CN.md`
- `config.example.toml`

Likely files to update:

- `core/engine.go`
- `cmd/cc-connect/main.go`
- config parsing structs and defaults
