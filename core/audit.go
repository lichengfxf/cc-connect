package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	AuditEventMessageUser         = "message.user"
	AuditEventMessageAgent        = "message.agent"
	AuditEventCommandExecuted     = "command.executed"
	AuditEventCommandBlocked      = "command.blocked"
	AuditEventPermissionRequested = "permission.requested"
	AuditEventPermissionApproved  = "permission.approved"
	AuditEventPermissionDenied    = "permission.denied"
	AuditEventAttachmentSent      = "attachment.sent"
	AuditEventSessionCreated      = "session.created"
	AuditEventSessionSwitched     = "session.switched"
	AuditEventSessionDeleted      = "session.deleted"
	AuditEventSessionRotated      = "session.rotated"
	AuditEventSessionCompressed   = "session.compressed"
)

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

type AuditFilter struct {
	Project    string
	UserID     string
	UserName   string
	Platform   string
	EventType  string
	Command    string
	Result     string
	SessionKey string
	SessionID  string
	Query      string
	From       time.Time
	To         time.Time
	Limit      int
	Offset     int
}

type AuditListResult struct {
	Items  []AuditEvent `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

type Auditor interface {
	Record(AuditEvent) error
	List(AuditFilter) (AuditListResult, error)
	Get(id string) (*AuditEvent, error)
	Export(filter AuditFilter, format string) ([]byte, string, error)
}

type noopAuditor struct{}

func (noopAuditor) Record(AuditEvent) error                            { return nil }
func (noopAuditor) List(AuditFilter) (AuditListResult, error)          { return AuditListResult{}, nil }
func (noopAuditor) Get(string) (*AuditEvent, error)                    { return nil, os.ErrNotExist }
func (noopAuditor) Export(AuditFilter, string) ([]byte, string, error) { return nil, "", nil }

type FileAuditor struct {
	dir string
	mu  sync.Mutex
}

func NewFileAuditor(dataDir string) (*FileAuditor, error) {
	dir := filepath.Join(dataDir, "audit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("audit: create dir: %w", err)
	}
	return &FileAuditor{dir: dir}, nil
}

func (a *FileAuditor) Record(event AuditEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d-%s-%s", event.Timestamp.UnixNano(), sanitizeAuditToken(event.Project), shortAuditID())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	path := filepath.Join(a.dir, "audit-"+event.Timestamp.Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("audit: open file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(event); err != nil {
		return fmt.Errorf("audit: encode event: %w", err)
	}
	return nil
}

func (a *FileAuditor) List(filter AuditFilter) (AuditListResult, error) {
	items, err := a.loadFiltered(filter)
	if err != nil {
		return AuditListResult{}, err
	}
	total := len(items)
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return AuditListResult{
		Items:  items[offset:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (a *FileAuditor) Get(id string) (*AuditEvent, error) {
	items, err := a.loadFiltered(AuditFilter{})
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			ev := items[i]
			return &ev, nil
		}
	}
	return nil, os.ErrNotExist
}

func (a *FileAuditor) Export(filter AuditFilter, format string) ([]byte, string, error) {
	items, err := a.loadFiltered(filter)
	if err != nil {
		return nil, "", err
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "jsonl":
		var b strings.Builder
		enc := json.NewEncoder(&b)
		for _, item := range items {
			if err := enc.Encode(item); err != nil {
				return nil, "", err
			}
		}
		return []byte(b.String()), "application/x-ndjson", nil
	case "csv":
		var b strings.Builder
		b.WriteString("timestamp,project,event_type,platform,user_id,user_name,chat_name,session_key,session_id,command,result,reason,role,content,content_sha256\n")
		for _, item := range items {
			row := []string{
				item.Timestamp.Format(time.RFC3339),
				item.Project,
				item.EventType,
				item.Platform,
				item.UserID,
				item.UserName,
				item.ChatName,
				item.SessionKey,
				item.SessionID,
				item.Command,
				item.Result,
				item.Reason,
				item.Role,
				item.Content,
				item.ContentSHA256,
			}
			for i, cell := range row {
				row[i] = csvQuote(cell)
			}
			b.WriteString(strings.Join(row, ","))
			b.WriteByte('\n')
		}
		return []byte(b.String()), "text/csv; charset=utf-8", nil
	default:
		return nil, "", fmt.Errorf("unsupported audit export format %q", format)
	}
}

func (a *FileAuditor) loadFiltered(filter AuditFilter) ([]AuditEvent, error) {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("audit: read dir: %w", err)
	}
	var items []AuditEvent
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(a.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("audit: read file %s: %w", entry.Name(), err)
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var ev AuditEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			if auditEventMatches(ev, filter) {
				items = append(items, ev)
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})
	return items, nil
}

func auditEventMatches(ev AuditEvent, f AuditFilter) bool {
	if f.Project != "" && ev.Project != f.Project {
		return false
	}
	if f.UserID != "" && !strings.EqualFold(ev.UserID, f.UserID) {
		return false
	}
	if f.UserName != "" && !strings.Contains(strings.ToLower(ev.UserName), strings.ToLower(f.UserName)) {
		return false
	}
	if f.Platform != "" && !strings.EqualFold(ev.Platform, f.Platform) {
		return false
	}
	if f.EventType != "" && ev.EventType != f.EventType {
		return false
	}
	if f.Command != "" && !strings.EqualFold(ev.Command, f.Command) {
		return false
	}
	if f.Result != "" && !strings.EqualFold(ev.Result, f.Result) {
		return false
	}
	if f.SessionKey != "" && ev.SessionKey != f.SessionKey {
		return false
	}
	if f.SessionID != "" && ev.SessionID != f.SessionID {
		return false
	}
	if !f.From.IsZero() && ev.Timestamp.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && ev.Timestamp.After(f.To) {
		return false
	}
	if f.Query != "" {
		q := strings.ToLower(f.Query)
		if !strings.Contains(strings.ToLower(ev.Content), q) &&
			!strings.Contains(strings.ToLower(ev.UserName), q) &&
			!strings.Contains(strings.ToLower(ev.ChatName), q) &&
			!strings.Contains(strings.ToLower(ev.Command), q) {
			return false
		}
	}
	return true
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(sk-[a-z0-9_\-]+)`),
	regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._\-]+`),
	regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[a-z0-9._\-]+`),
}

func RedactSecrets(text string) string {
	out := text
	for _, re := range secretPatterns {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			sub := re.FindStringSubmatch(m)
			if len(sub) >= 2 {
				prefix := sub[1]
				if strings.EqualFold(prefix, m) {
					return prefix
				}
				return prefix + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return out
}

func PrepareAuditContent(content string, maxBytes int) (string, string, bool) {
	hash := sha256.Sum256([]byte(content))
	sha := hex.EncodeToString(hash[:])
	redacted := RedactSecrets(content)
	if maxBytes <= 0 || len(redacted) <= maxBytes {
		return redacted, sha, false
	}
	return redacted[:maxBytes], sha, true
}

func csvQuote(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + strings.ReplaceAll(s, "\n", " ") + `"`
}

func sanitizeAuditToken(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "audit"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortAuditID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return hex.EncodeToString(sum[:])[:8]
}
