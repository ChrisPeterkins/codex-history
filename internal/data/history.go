package data

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// readHistoryIndex loads Codex's lightweight prompt index by session ID.
func readHistoryIndex() map[string][]HistoryEntry {
	result := make(map[string][]HistoryEntry)
	f, err := os.Open(filepath.Join(codexDir, "history.jsonl"))
	if err != nil {
		return result
	}
	defer f.Close()
	scanner := newScanner(f, false)
	for scanner.Scan() {
		var row struct {
			SessionID string `json:"session_id"`
			Text      string `json:"text"`
			Timestamp int64  `json:"ts"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil || row.SessionID == "" || strings.TrimSpace(row.Text) == "" {
			continue
		}
		result[row.SessionID] = append(result[row.SessionID], HistoryEntry{
			Display: row.Text, Timestamp: row.Timestamp, SessionID: row.SessionID,
		})
	}
	for id := range result {
		sort.Slice(result[id], func(i, j int) bool { return result[id][i].Timestamp < result[id][j].Timestamp })
	}
	return result
}

func historyOnlyProject(index map[string][]HistoryEntry, seen map[string]bool) *Project {
	p := &Project{Name: "History only", DirName: "history", HistoryOnly: true}
	for id, entries := range index {
		if seen[id] || len(entries) == 0 {
			continue
		}
		p.Sessions = append(p.Sessions, Session{
			ID: id, StartedAt: unixTime(entries[0].Timestamp), Preview: truncate(entries[0].Display, maxPreviewLen),
			MessageCount: len(entries), HistoryOnly: true, HistoryEntries: entries,
		})
	}
	if len(p.Sessions) == 0 {
		return nil
	}
	sort.Slice(p.Sessions, func(i, j int) bool { return p.Sessions[i].StartedAt.After(p.Sessions[j].StartedAt) })
	p.SessionCount, p.LastActive = len(p.Sessions), p.Sessions[0].StartedAt
	return p
}

// LoadHistory returns prompt-only sessions whose full Codex records are gone.
func LoadHistory() ([]Project, error) {
	p := historyOnlyProject(readHistoryIndex(), map[string]bool{})
	if p == nil {
		return nil, nil
	}
	return []Project{*p}, nil
}

// LoadHistoryMessages converts history index entries into user messages.
func LoadHistoryMessages(session *Session) ([]Message, error) {
	var messages []Message
	for i, entry := range session.HistoryEntries {
		text := strings.TrimSpace(entry.Display)
		if text == "" {
			continue
		}
		messages = append(messages, Message{
			UUID: entry.SessionID + ":" + strconv.Itoa(i),
			Type: "user", Role: "user", RawText: text, Timestamp: unixTime(entry.Timestamp), SessionID: entry.SessionID,
			ContentBlocks: []ContentBlock{{Type: "text", Text: text}},
		})
	}
	return messages, nil
}

type rolloutMeta struct {
	id, cwd, preview string
	started          time.Time
	subagent         bool
}

func rolloutID(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if len(base) >= 36 {
		return base[len(base)-36:]
	}
	return base
}

func peekRollout(path string) rolloutMeta {
	var result rolloutMeta
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()
	scanner := newScanner(f, true)
	for scanner.Scan() {
		var row struct {
			Type, Timestamp string
			Payload         json.RawMessage
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			continue
		}
		if row.Type == "session_meta" {
			var meta struct {
				ID, SessionID, Cwd, Timestamp string
				Source                        json.RawMessage
			}
			if json.Unmarshal(row.Payload, &meta) == nil {
				result.id, result.cwd = meta.ID, meta.Cwd
				if result.id == "" {
					result.id = meta.SessionID
				}
				stamp := meta.Timestamp
				if stamp == "" {
					stamp = row.Timestamp
				}
				result.started, _ = time.Parse(time.RFC3339Nano, stamp)
				result.subagent = strings.Contains(string(meta.Source), `"subagent"`)
			}
		}
		if result.preview == "" {
			for _, msg := range parseRolloutLine(scanner.Bytes()) {
				if msg.Type == "user" && msg.RawText != "" {
					result.preview = msg.RawText
					break
				}
			}
		}
		if result.id != "" && result.preview != "" {
			break
		}
	}
	if result.id == "" {
		result.id = rolloutID(filepath.Base(path))
	}
	return result
}
