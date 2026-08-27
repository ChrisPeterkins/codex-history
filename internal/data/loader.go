package data

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chrispeterkins/codex-history/internal/config"
	_ "modernc.org/sqlite"
)

var codexDir string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	codexDir = filepath.Join(home, ".codex")
}

func newScanner(f *os.File, large bool) *bufio.Scanner {
	s := bufio.NewScanner(f)
	if large {
		s.Buffer(make([]byte, scannerInitBuf), scannerLargeBuf)
	} else {
		s.Buffer(make([]byte, scannerInitBuf), scannerMaxBuf)
	}
	return s
}

func openCodexDB(name string) (*sql.DB, error) {
	path := filepath.Join(codexDir, name)
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// LoadProjects builds the project/session catalog from Codex's read-only
// SQLite projection, then fills any gaps from rollout and history JSONL files.
func LoadProjects() ([]Project, error) {
	projectMap := make(map[string]*Project)
	seen := make(map[string]bool)
	history := readHistoryIndex()

	db, dbErr := openCodexDB("state_5.sqlite")
	catalogErr := dbErr
	if dbErr == nil {
		defer db.Close()
		rows, err := db.Query(`SELECT id, rollout_path, created_at, updated_at, cwd,
            title, tokens_used FROM threads
            WHERE source NOT LIKE '{"subagent"%'
            ORDER BY created_at DESC`)
		if err == nil {
			catalogErr = nil
			defer rows.Close()
			stats := loadTurnStats()
			for rows.Next() {
				var id, rollout, cwd, title string
				var created, updated, tokens int64
				if rows.Scan(&id, &rollout, &created, &updated, &cwd, &title, &tokens) != nil {
					continue
				}
				preview := title
				if entries := history[id]; len(entries) > 0 {
					preview = entries[0].Display
				}
				st := stats[id]
				s := Session{ID: id, StartedAt: unixTime(created), Preview: truncate(preview, maxPreviewLen),
					FilePath: rollout, MessageCount: st.messages, TotalTokensOut: clampInt64(tokens), TotalDurationMs: st.duration,
					HistoryEntries: history[id]}
				if info, err := os.Stat(rollout); err == nil {
					s.FileSize = info.Size()
				}
				addSession(projectMap, cwd, s)
				seen[id] = true
			}
			catalogErr = rows.Err()
		} else {
			catalogErr = err
		}
	}

	// Rollouts are authoritative for new sessions that have not reached the DB
	// projection yet. Known IDs are skipped without opening their large files.
	for _, root := range []string{filepath.Join(codexDir, "sessions"), filepath.Join(codexDir, "archived_sessions")} {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			id := rolloutID(d.Name())
			if seen[id] {
				return nil
			}
			meta := peekRollout(path)
			if meta.id == "" || meta.subagent {
				return nil
			}
			info, _ := d.Info()
			s := Session{ID: meta.id, StartedAt: meta.started, Preview: truncate(meta.preview, maxPreviewLen), FilePath: path}
			if info != nil {
				s.FileSize = info.Size()
			}
			addSession(projectMap, meta.cwd, s)
			seen[meta.id] = true
			return nil
		})
	}

	// history.jsonl can outlive both a rollout and its catalog row.
	if p := historyOnlyProject(history, seen); p != nil {
		projectMap["\x00history"] = p
	}
	if len(projectMap) == 0 && catalogErr != nil {
		return nil, fmt.Errorf("could not load Codex history from %s: %w", codexDir, catalogErr)
	}

	projects := make([]Project, 0, len(projectMap))
	for _, p := range projectMap {
		sort.Slice(p.Sessions, func(i, j int) bool { return p.Sessions[i].StartedAt.After(p.Sessions[j].StartedAt) })
		p.SessionCount = len(p.Sessions)
		if p.SessionCount > 0 {
			p.LastActive = p.Sessions[0].StartedAt
		}
		projects = append(projects, *p)
	}
	sort.Slice(projects, func(i, j int) bool { return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name) })
	for i := range projects {
		for j := range projects[i].Sessions {
			projects[i].Sessions[j].Project = &projects[i]
		}
	}
	return projects, nil
}

type turnStat struct{ messages, duration int }

func loadTurnStats() map[string]turnStat {
	result := make(map[string]turnStat)
	db, err := openCodexDB("thread_history_1.sqlite")
	if err != nil {
		return result
	}
	defer db.Close()
	rows, err := db.Query(`SELECT thread_id, COUNT(*) * 2, COALESCE(SUM(duration_ms), 0)
        FROM thread_turns GROUP BY thread_id`)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var s turnStat
		if rows.Scan(&id, &s.messages, &s.duration) == nil {
			result[id] = s
		}
	}
	return result
}

func addSession(projects map[string]*Project, cwd string, session Session) {
	key := filepath.Clean(cwd)
	if cwd == "" || key == "." {
		key = "(no project)"
	}
	p := projects[key]
	if p == nil {
		name := projectNameFromPath(cwd)
		if name == "" {
			name = "No project"
		}
		p = &Project{Name: name, Path: cwd, DirName: key}
		projects[key] = p
	}
	p.Sessions = append(p.Sessions, session)
}

func projectNameFromPath(path string) string {
	clean := strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
	parts := strings.Split(clean, "/")
	roots := config.ProjectRoots()
	for i, part := range parts {
		if roots[part] && i+1 < len(parts) {
			return strings.Join(parts[i+1:], " ")
		}
	}
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// LoadSessions returns the already indexed sessions for a project.
func LoadSessions(project *Project) ([]Session, error) {
	sessions := append([]Session(nil), project.Sessions...)
	for i := range sessions {
		sessions[i].Project = project
	}
	return sessions, nil
}

// SearchSessionIDs searches full chat text plus the command/metadata prefix of tool items.
func SearchSessionIDs(query string, limit int) (map[string]bool, error) {
	db, err := openCodexDB("thread_history_1.sqlite")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query = strings.ToLower(query)
	rows, err := db.Query(`SELECT DISTINCT thread_id FROM thread_items WHERE
        (item_type IN ('userMessage','agentMessage','reasoning') AND instr(lower(item_json), ?) > 0) OR
        (item_type IN ('commandExecution','mcpToolCall','fileChange') AND instr(lower(substr(item_json,1,1024)), ?) > 0)
        LIMIT ?`, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			result[id] = true
		}
	}
	return result, rows.Err()
}

// ConversationRevision returns a cheap change token without loading message bodies.
func ConversationRevision(session *Session) (string, error) {
	var sourceErr error
	if db, err := openCodexDB("thread_history_1.sqlite"); err == nil {
		defer db.Close()
		var count, ordinal int64
		err = db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(updated_at_ordinal),0)
            FROM thread_items WHERE thread_id = ?`, session.ID).Scan(&count, &ordinal)
		if err == nil && count > 0 {
			return fmt.Sprintf("db:%d:%d", count, ordinal), nil
		}
		sourceErr = err
	} else {
		sourceErr = err
	}
	if info, err := os.Stat(session.FilePath); err == nil {
		return fmt.Sprintf("file:%d:%d", info.Size(), info.ModTime().UnixNano()), nil
	}
	if n := len(session.HistoryEntries); n > 0 {
		return fmt.Sprintf("history:%d:%d", n, session.HistoryEntries[n-1].Timestamp), nil
	}
	if sourceErr == nil {
		sourceErr = errors.New("conversation source is unavailable")
	}
	return "", sourceErr
}

// LoadMessages prefers Codex's complete projected history and falls back to a
// raw rollout, then the lightweight prompt-only history.
func LoadMessages(session *Session) ([]Message, error) {
	var loadErrs []error
	if messages, err := loadProjectedMessages(session.ID); err == nil && len(messages) > 0 {
		PairToolInteractions(messages)
		return messages, nil
	} else if err != nil {
		loadErrs = append(loadErrs, fmt.Errorf("projected history: %w", err))
	}
	if session.FilePath != "" {
		if messages, err := loadRolloutMessages(session.FilePath); err == nil && len(messages) > 0 {
			PairToolInteractions(messages)
			return messages, nil
		} else if err != nil {
			loadErrs = append(loadErrs, fmt.Errorf("rollout: %w", err))
		}
	}
	if len(session.HistoryEntries) > 0 {
		return LoadHistoryMessages(session)
	}
	loadErrs = append(loadErrs, errors.New("conversation content is unavailable"))
	return nil, errors.Join(loadErrs...)
}

func loadProjectedMessages(threadID string) ([]Message, error) {
	db, err := openCodexDB("thread_history_1.sqlite")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT item_id, created_at_ms, item_type, item_json
        FROM thread_items WHERE thread_id = ? ORDER BY rollout_ordinal`, threadID)
	if err != nil {
		return nil, err
	}
	var messages []Message
	for rows.Next() {
		var id, itemType string
		var created int64
		var raw []byte
		if rows.Scan(&id, &created, &itemType, &raw) == nil {
			messages = append(messages, parseProjectedItem(id, itemType, raw, time.UnixMilli(created))...)
		}
	}
	rowErr := rows.Err()
	_ = rows.Close()
	turns, _ := db.Query(`SELECT completed_at, duration_ms FROM thread_turns
        WHERE thread_id = ? AND duration_ms > 0 ORDER BY rollout_ordinal`, threadID)
	if turns != nil {
		defer turns.Close()
		for turns.Next() {
			var completed, duration int64
			if turns.Scan(&completed, &duration) == nil {
				messages = append(messages, Message{Type: "system", Subtype: "turn_duration",
					Timestamp: unixTime(completed), DurationMs: clampInt64(duration)})
			}
		}
	}
	sort.SliceStable(messages, func(i, j int) bool { return messages[i].Timestamp.Before(messages[j].Timestamp) })
	return dedupeMessages(messages), rowErr
}

func parseProjectedItem(id, itemType string, raw []byte, ts time.Time) []Message {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil {
		return nil
	}
	switch itemType {
	case "userMessage":
		return textMessage(id, "user", contentText(item["content"]), ts)
	case "agentMessage":
		return textMessage(id, "assistant", rawString(item["text"]), ts)
	case "reasoning":
		thinking := reasoningText(item["summary"], item["content"])
		if thinking != "" {
			return []Message{{UUID: id, Type: "assistant", Timestamp: ts,
				ContentBlocks: []ContentBlock{{Type: "thinking", Thinking: thinking}}}}
		}
	case "commandExecution":
		input := map[string]interface{}{"command": rawString(item["command"]), "cwd": rawString(item["cwd"])}
		return toolMessages(id, "Bash", input, rawString(item["aggregatedOutput"]), failedStatus(item["status"]), ts)
	case "mcpToolCall":
		input := rawMap(item["arguments"])
		name := rawString(item["tool"])
		if server := rawString(item["server"]); server != "" {
			name = server + "." + name
		}
		result := prettyRaw(item["result"])
		if errText := prettyRaw(item["error"]); errText != "" && errText != "null" {
			result = errText
		}
		return toolMessages(id, name, input, result, failedStatus(item["status"]), ts)
	case "fileChange":
		var changes []struct{ Path, Kind, Diff string }
		_ = json.Unmarshal(item["changes"], &changes)
		var out []Message
		for i, change := range changes {
			toolID := fmt.Sprintf("%s-%d", id, i)
			input := map[string]interface{}{"file_path": change.Path, "diff": change.Diff, "kind": change.Kind}
			out = append(out, toolMessages(toolID, "Edit", input, rawString(item["status"]), failedStatus(item["status"]), ts)...)
		}
		return out
	case "webSearch":
		return toolMessages(id, "WebSearch", map[string]interface{}{"query": rawString(item["query"])}, prettyRaw(item["results"]), false, ts)
	case "imageView":
		return toolMessages(id, "ViewImage", map[string]interface{}{"path": rawString(item["path"])}, "", false, ts)
	}
	return nil
}

func textMessage(id, role, text string, ts time.Time) []Message {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []Message{{UUID: id, Type: role, Role: role, RawText: text, Timestamp: ts,
		ContentBlocks: []ContentBlock{{Type: "text", Text: text}}}}
}

func toolMessages(id, name string, input map[string]interface{}, output string, failed bool, ts time.Time) []Message {
	return []Message{
		{UUID: id, Type: "assistant", Timestamp: ts, ContentBlocks: []ContentBlock{{Type: "tool_use", ToolID: id, ToolName: name, Input: input}}},
		{UUID: id + "-result", Type: "user", Timestamp: ts.Add(time.Nanosecond), ContentBlocks: []ContentBlock{{Type: "tool_result", ToolUseID: id, Content: output, IsError: failed}}},
	}
}

func loadRolloutMessages(path string) ([]Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var messages []Message
	scanner := newScanner(f, true)
	for scanner.Scan() {
		messages = append(messages, parseRolloutLine(scanner.Bytes())...)
	}
	return dedupeMessages(messages), scanner.Err()
}

func parseRolloutLine(line []byte) []Message {
	var row struct {
		Type, Timestamp string
		Ordinal         int
		Payload         json.RawMessage
	}
	if json.Unmarshal(line, &row) != nil {
		return nil
	}
	ts, _ := time.Parse(time.RFC3339Nano, row.Timestamp)
	var p map[string]json.RawMessage
	if json.Unmarshal(row.Payload, &p) != nil {
		return nil
	}
	kind := rawString(p["type"])
	id := rawString(p["id"])
	if id == "" {
		id = fmt.Sprintf("%d", row.Ordinal)
	}
	if row.Type == "response_item" {
		switch kind {
		case "message":
			role := rawString(p["role"])
			if role == "user" || role == "assistant" {
				return textMessage(id, role, contentText(p["content"]), ts)
			}
		case "reasoning":
			thinking := reasoningText(p["summary"], p["content"])
			if thinking != "" {
				return []Message{{UUID: id, Type: "assistant", Timestamp: ts, ContentBlocks: []ContentBlock{{Type: "thinking", Thinking: thinking}}}}
			}
		case "function_call", "custom_tool_call":
			callID := rawString(p["call_id"])
			if callID == "" {
				callID = id
			}
			return toolMessages(callID, rawString(p["name"]), rawMap(p["arguments"]), "", false, ts)[:1]
		case "function_call_output", "custom_tool_call_output":
			callID := rawString(p["call_id"])
			return toolMessages(callID, "", nil, outputText(p["output"]), false, ts)[1:]
		}
	}
	if row.Type == "event_msg" {
		switch kind {
		case "user_message":
			return textMessage(id, "user", rawString(p["message"]), ts)
		case "agent_message":
			return textMessage(id, "assistant", rawString(p["message"]), ts)
		case "turn_completed":
			var duration int
			_ = json.Unmarshal(p["duration_ms"], &duration)
			return []Message{{UUID: id, Type: "system", Subtype: "turn_duration", DurationMs: duration, Timestamp: ts}}
		}
	}
	return nil
}

// PairToolInteractions attaches results to the assistant message that invoked them.
func PairToolInteractions(messages []Message) {
	uses := make(map[string]*Message)
	blocks := make(map[string]ContentBlock)
	for i := range messages {
		for _, block := range messages[i].ContentBlocks {
			if block.Type == "tool_use" && block.ToolID != "" {
				uses[block.ToolID], blocks[block.ToolID] = &messages[i], block
			}
		}
	}
	for _, msg := range messages {
		for _, block := range msg.ContentBlocks {
			if use := uses[block.ToolUseID]; block.Type == "tool_result" && use != nil {
				use.ToolPairs = append(use.ToolPairs, ToolInteraction{Use: blocks[block.ToolUseID], Result: block, Name: blocks[block.ToolUseID].ToolName})
			}
		}
	}
}

func contentText(raw json.RawMessage) string {
	var blocks []struct{ Type, Text string }
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, block := range blocks {
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return rawString(raw)
}

func reasoningText(raws ...json.RawMessage) string {
	var parts []string
	for _, raw := range raws {
		var blocks []map[string]interface{}
		if json.Unmarshal(raw, &blocks) != nil {
			continue
		}
		for _, block := range blocks {
			if text, ok := block["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func rawString(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func rawMap(raw json.RawMessage) map[string]interface{} {
	var result map[string]interface{}
	if json.Unmarshal(raw, &result) == nil {
		return result
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		_ = json.Unmarshal([]byte(encoded), &result)
	}
	return result
}

func outputText(raw json.RawMessage) string {
	if s := rawString(raw); s != "" {
		return s
	}
	return prettyRaw(raw)
}

func prettyRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value interface{}
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return ""
	}
	b, _ := json.MarshalIndent(value, "", "  ")
	return string(b)
}

func failedStatus(raw json.RawMessage) bool {
	s := strings.ToLower(rawString(raw))
	return s == "failed" || s == "error" || s == "declined"
}

func dedupeMessages(messages []Message) []Message {
	result := messages[:0]
	for _, msg := range messages {
		if len(result) > 0 && msg.RawText != "" {
			prev := result[len(result)-1]
			if prev.Type == msg.Type && prev.RawText == msg.RawText {
				continue
			}
		}
		result = append(result, msg)
	}
	return result
}

func unixTime(value int64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value)
	}
	return time.Unix(value, 0)
}

func clampInt64(n int64) int {
	maxInt := int64(^uint(0) >> 1)
	if n > maxInt {
		return int(maxInt)
	}
	return int(n)
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
