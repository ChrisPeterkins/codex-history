package data

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSearchSessionIDsSearchesFullContent(t *testing.T) {
	oldDir := codexDir
	codexDir = t.TempDir()
	t.Cleanup(func() { codexDir = oldDir })
	db, err := sql.Open("sqlite", filepath.Join(codexDir, "thread_history_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE thread_items (thread_id TEXT, item_json BLOB, item_type TEXT, updated_at_ordinal INTEGER);
		INSERT INTO thread_items VALUES ('matching-session', '{"text":"buried unique phrase"}', 'agentMessage', 7), ('tool-session', '{"command":"unique phrase tool"}', 'commandExecution', 2), ('other', '{"text":"unrelated"}', 'agentMessage', 1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	matches, err := SearchSessionIDs("UNIQUE PHRASE", 20)
	if err != nil || !matches["matching-session"] || !matches["tool-session"] || matches["other"] {
		t.Fatalf("matches=%v err=%v", matches, err)
	}
	if revision, err := ConversationRevision(&Session{ID: "matching-session"}); err != nil || revision != "db:1:7" {
		t.Fatalf("revision=%q err=%v", revision, err)
	}
	if _, err := LoadProjects(); err == nil {
		t.Fatal("missing catalog did not return a visible error")
	}
}
