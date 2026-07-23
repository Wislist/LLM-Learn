package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wislist/llmg"
	_ "modernc.org/sqlite"
)

type Session struct {
	ID               string
	Title            string
	Model            string
	CreatedAt        string
	UpdatedAt        string
	PromptTokens     int
	CompletionTokens int
}

type SessionStore struct {
	db *sql.DB
}

func OpenSessionStore(path string) (*SessionStore, error) {
	if dir := filepath.Dir(path); dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open session db: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &SessionStore{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SessionStore) Close() error { return s.db.Close() }

func (s *SessionStore) init() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,
  title      TEXT,
  model      TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  prompt_tokens INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS messages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES sessions(id),
  seq        INTEGER NOT NULL,
  payload    TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, seq);
CREATE TABLE IF NOT EXISTS session_summaries (
  session_id         TEXT PRIMARY KEY REFERENCES sessions(id),
  summary            TEXT NOT NULL,
  covered_message_id INTEGER NOT NULL DEFAULT 0,
  updated_at         DATETIME DEFAULT CURRENT_TIMESTAMP
);
`)
	if err != nil {
		return err
	}
	// 兼容旧库：补列（已存在则忽略错误）
	s.db.Exec("ALTER TABLE sessions ADD COLUMN prompt_tokens INTEGER NOT NULL DEFAULT 0")
	s.db.Exec("ALTER TABLE sessions ADD COLUMN completion_tokens INTEGER NOT NULL DEFAULT 0")
	return nil
}

func (s *SessionStore) CreateSession(title, model string) (string, error) {
	id := newSessionID()
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, title, model, updated_at) VALUES (?, ?, ?, ?)`,
		id, title, model, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return id, nil
}

func (s *SessionStore) RenameSession(id, title string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET title=?, updated_at=? WHERE id=?`,
		title, time.Now().Format(time.RFC3339), id,
	)
	return err
}

func (s *SessionStore) AppendMessage(sessionID string, msg llmg.Message) (int64, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("marshal message: %w", err)
	}
	result, err := s.db.Exec(
		`INSERT INTO messages (session_id, seq, payload)
		 VALUES (?, COALESCE((SELECT MAX(seq) + 1 FROM messages WHERE session_id=?), 1), ?)`,
		sessionID, sessionID, string(payload),
	)
	if err != nil {
		return 0, fmt.Errorf("append message: %w", err)
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("message id: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE sessions SET updated_at=? WHERE id=?`,
		time.Now().Format(time.RFC3339), sessionID,
	)
	return messageID, err
}

// AddUsage 累加 session 的 token 用量。
func (s *SessionStore) AddUsage(sessionID string, prompt, completion int) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET prompt_tokens=prompt_tokens+?, completion_tokens=completion_tokens+?, updated_at=? WHERE id=?`,
		prompt, completion, time.Now().Format(time.RFC3339), sessionID,
	)
	return err
}

func (s *SessionStore) LoadMessages(sessionID string) ([]llmg.Message, error) {
	_, messages, _, err := s.LoadContext(sessionID)
	return messages, err
}

func (s *SessionStore) LoadContext(sessionID string) (string, []llmg.Message, []int64, error) {
	var summary string
	var coveredID int64
	err := s.db.QueryRow(
		`SELECT summary, covered_message_id FROM session_summaries WHERE session_id=?`,
		sessionID,
	).Scan(&summary, &coveredID)
	if err != nil && err != sql.ErrNoRows {
		return "", nil, nil, fmt.Errorf("load summary: %w", err)
	}

	rows, err := s.db.Query(
		`SELECT id, payload FROM messages WHERE session_id=? AND id>? ORDER BY id ASC`,
		sessionID, coveredID,
	)
	if err != nil {
		return "", nil, nil, fmt.Errorf("load messages: %w", err)
	}
	defer rows.Close()

	var out []llmg.Message
	var ids []int64
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return "", nil, nil, err
		}
		var msg llmg.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			return "", nil, nil, fmt.Errorf("unmarshal message: %w", err)
		}
		ids = append(ids, id)
		out = append(out, msg)
	}
	return summary, out, ids, rows.Err()
}

func (s *SessionStore) SaveSummary(sessionID, summary string, coveredMessageID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO session_summaries (session_id, summary, covered_message_id, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		 summary=excluded.summary,
		 covered_message_id=excluded.covered_message_id,
		 updated_at=excluded.updated_at`,
		sessionID, summary, coveredMessageID, time.Now().Format(time.RFC3339),
	)
	return err
}

func (s *SessionStore) LatestSession() (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, title, model, created_at, updated_at, prompt_tokens, completion_tokens
		 FROM sessions ORDER BY updated_at DESC LIMIT 1`,
	)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.CreatedAt, &sess.UpdatedAt, &sess.PromptTokens, &sess.CompletionTokens)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStore) ListSessions(limit int) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, title, model, created_at, updated_at, prompt_tokens, completion_tokens
		 FROM sessions ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.CreatedAt, &sess.UpdatedAt, &sess.PromptTokens, &sess.CompletionTokens); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *SessionStore) GetSession(id string) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, title, model, created_at, updated_at, prompt_tokens, completion_tokens FROM sessions WHERE id=?`,
		id,
	)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Title, &sess.Model, &sess.CreatedAt, &sess.UpdatedAt, &sess.PromptTokens, &sess.CompletionTokens)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStore) MessageCount(sessionID string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE session_id=?`, sessionID,
	).Scan(&n)
	return n, err
}

func newSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
