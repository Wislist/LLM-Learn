package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MemoryKind 分类长期记忆。
type MemoryKind string

const (
	MemPreference  MemoryKind = "preference"
	MemProject     MemoryKind = "project"
	MemDecision    MemoryKind = "decision"
	MemConstraint  MemoryKind = "constraint"
	MemTask        MemoryKind = "task"
)

// Memory 表示一条长期记忆条目。
type Memory struct {
	ID        int64
	Scope     string
	Kind      MemoryKind
	Content   string
	Confidence float64
	CreatedAt string
}

// MemoryStore 管理长期记忆的存储和检索。
type MemoryStore struct {
	db     *sql.DB
	useFTS bool
}

func (s *SessionStore) MemoryStore() *MemoryStore {
	return &MemoryStore{db: s.db, useFTS: s.hasFTS()}
}

func (s *SessionStore) hasFTS() bool {
	var result struct{ Val string }
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='memories_fts' LIMIT 1`,
	).Scan(&result.Val)
	return err == nil
}

// InitMemoryTables 创建记忆表和 FTS5 索引。
func (s *SessionStore) InitMemoryTables() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS memories (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  scope       TEXT NOT NULL DEFAULT 'global',
  kind        TEXT NOT NULL,
  content     TEXT NOT NULL,
  source_session_id TEXT,
  confidence  REAL NOT NULL DEFAULT 1.0,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
CREATE INDEX IF NOT EXISTS idx_memories_kind ON memories(kind);
`)
	if err != nil {
		return fmt.Errorf("create memories table: %w", err)
	}

	// FTS5 可能在某些 SQLite 构建中不可用，失败时降级。
	_, ftsErr := s.db.Exec(`
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
  content,
  content='memories',
  content_rowid='id'
);
`)
	if ftsErr != nil {
		// FTS5 不可用，创建普通索引作为降级方案。
		s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_content ON memories(content)`)
	}
	return nil
}

func (m *MemoryStore) Save(scope string, kind MemoryKind, content, sourceSessionID string) error {
	// 去重：同 scope + kind + content 已存在则更新时间。
	var existingID int64
	err := m.db.QueryRow(
		`SELECT id FROM memories WHERE scope=? AND kind=? AND content=? LIMIT 1`,
		scope, string(kind), content,
	).Scan(&existingID)
	if err == nil {
		_, err := m.db.Exec(
			`UPDATE memories SET updated_at=?, last_used_at=? WHERE id=?`,
			time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339), existingID,
		)
		return err
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check existing memory: %w", err)
	}

	result, err := m.db.Exec(
		`INSERT INTO memories (scope, kind, content, source_session_id, confidence, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1.0, ?, ?)`,
		scope, string(kind), content, sourceSessionID,
		time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert memory: %w", err)
	}

	if m.useFTS {
		id, _ := result.LastInsertId()
		m.db.Exec(`INSERT INTO memories_fts (rowid, content) VALUES (?, ?)`, id, content)
	}
	return nil
}

func (m *MemoryStore) Delete(id int64) error {
	_, err := m.db.Exec(`DELETE FROM memories WHERE id=?`, id)
	if err != nil {
		return err
	}
	if m.useFTS {
		m.db.Exec(`DELETE FROM memories_fts WHERE rowid=?`, id)
	}
	return nil
}

// Search 根据关键词检索最相关的记忆。
// 使用 FTS5 全文搜索（如可用），否则降级为 LIKE 匹配。
func (m *MemoryStore) Search(query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}

	if m.useFTS && strings.TrimSpace(query) != "" {
		return m.searchFTS(query, limit)
	}
	return m.searchLike(query, limit)
}

func (m *MemoryStore) searchFTS(query string, limit int) ([]Memory, error) {
	// 对用户查询做简单的 FTS5 安全转义。
	ftsQuery := sanitizeFTSQuery(query)
	if ftsQuery == "" {
		return m.searchLike(query, limit)
	}

	rows, err := m.db.Query(
		`SELECT m.id, m.scope, m.kind, m.content, m.confidence, m.created_at
		 FROM memories m
		 JOIN memories_fts f ON m.id = f.rowid
		 WHERE memories_fts MATCH ?
		 ORDER BY rank, m.updated_at DESC
		 LIMIT ?`,
		ftsQuery, limit,
	)
	if err != nil {
		// FTS 查询失败时降级。
		return m.searchLike(query, limit)
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (m *MemoryStore) searchLike(query string, limit int) ([]Memory, error) {
	pattern := "%" + query + "%"
	rows, err := m.db.Query(
		`SELECT id, scope, kind, content, confidence, created_at
		 FROM memories
		 WHERE content LIKE ?
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemories(rows)
}

func (m *MemoryStore) ListAll(limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.db.Query(
		`SELECT id, scope, kind, content, confidence, created_at
		 FROM memories ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemories(rows)
}

func scanMemories(rows *sql.Rows) ([]Memory, error) {
	var out []Memory
	for rows.Next() {
		var mem Memory
		if err := rows.Scan(&mem.ID, &mem.Scope, &mem.Kind, &mem.Content, &mem.Confidence, &mem.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, mem)
	}
	return out, rows.Err()
}

// sanitizeFTSQuery 将用户查询转成安全的 FTS5 MATCH 表达式。
// 简单方案：按空格分词，每个词加引号做短语匹配，用 OR 连接。
func sanitizeFTSQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	words := strings.Fields(query)
	var parts []string
	for _, w := range words {
		// 去掉 FTS5 特殊字符。
		w = strings.Map(func(r rune) rune {
			if r == '"' || r == '*' || r == '(' || r == ')' || r == ':' {
				return -1
			}
			return r
		}, w)
		if w != "" {
			parts = append(parts, `"`+w+`"`)
		}
	}
	return strings.Join(parts, " OR ")
}
