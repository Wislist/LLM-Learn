package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestGlob_RecursiveDoubleStar(t *testing.T) {
	dir := t.TempDir()
	// 建几个 .go 文件在不同层级
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "sub", "b.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "sub", "deep", "c.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "d.txt"), "hello\n")

	tool := &globTool{}
	out, err := tool.Execute(mustJSON(t, map[string]any{
		"pattern": "**/*.go", "path": dir,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Matches []string `json:"matches"`
		Count   int      `json:"count"`
	}
	json.Unmarshal([]byte(out), &res)
	if res.Count != 3 {
		t.Errorf("got %d matches, want 3: %v", res.Count, res.Matches)
	}
	// 确认不含 .txt
	for _, m := range res.Matches {
		if filepath.Ext(m) == ".txt" {
			t.Errorf("should not match .txt: %s", m)
		}
	}
}

func TestGlob_SimplePattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "x\n")
	writeFile(t, filepath.Join(dir, "sub", "b.go"), "x\n")

	tool := &globTool{}
	out, _ := tool.Execute(mustJSON(t, map[string]any{
		"pattern": "*.go", "path": dir,
	}))
	var res struct{ Count int }
	json.Unmarshal([]byte(out), &res)
	// *.go 只匹配当前层，不递归
	if res.Count != 1 {
		t.Errorf("got %d, want 1 (current dir only)", res.Count)
	}
}

func TestGrep_RegexSearch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.go"), "package main\nfunc foo() {}\nfunc bar() {}\n")
	writeFile(t, filepath.Join(dir, "b.go"), "package main\nconst x = 1\n")

	tool := &grepTool{}
	out, err := tool.Execute(mustJSON(t, map[string]any{
		"pattern": "func \\w+", "path": dir, "include": "*.go",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Hits  []grepHit `json:"hits"`
		Count int       `json:"count"`
	}
	json.Unmarshal([]byte(out), &res)
	if res.Count != 2 {
		t.Errorf("got %d hits, want 2: %+v", res.Count, res.Hits)
	}
	for _, h := range res.Hits {
		if !contains(h.Content, "func ") {
			t.Errorf("hit content should contain 'func ': %s", h.Content)
		}
	}
}

func TestGrep_MaxResults(t *testing.T) {
	dir := t.TempDir()
	// 写一个有 10 行匹配的文件
	content := ""
	for i := 0; i < 10; i++ {
		content += "TODO something\n"
	}
	writeFile(t, filepath.Join(dir, "a.txt"), content)

	tool := &grepTool{}
	out, _ := tool.Execute(mustJSON(t, map[string]any{
		"pattern": "TODO", "path": dir, "max_results": 3,
	}))
	var res struct {
		Count     int  `json:"count"`
		Truncated bool `json:"truncated"`
	}
	json.Unmarshal([]byte(out), &res)
	if res.Count != 3 {
		t.Errorf("got %d, want 3", res.Count)
	}
	if !res.Truncated {
		t.Error("should be truncated")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(s[0:len(sub)] == sub) || contains(s[1:], sub))
}
