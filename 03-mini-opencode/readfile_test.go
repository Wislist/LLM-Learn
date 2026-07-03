package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile_OffsetLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	content := strings.Repeat("line\n", 100) // 100 行（Split 后 101 元素，含尾空行）
	writeFile(t, p, content)

	tool := &readFileTool{}
	out, err := tool.Execute(mustJSON(t, map[string]any{
		"path": p, "offset": 10, "limit": 5,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Lines      int    `json:"lines"`
		TotalLines int    `json:"total_lines"`
		Offset     int    `json:"offset"`
		Content    string `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Lines != 5 {
		t.Errorf("lines: got %d want 5", res.Lines)
	}
	// Split("line\n...\n", "\n") 产生 101 个元素（含尾空行）
	if res.TotalLines != 101 {
		t.Errorf("total_lines: got %d want 101", res.TotalLines)
	}
	if res.Offset != 10 {
		t.Errorf("offset: got %d want 10", res.Offset)
	}
	// 应包含第 10..14 行，不含第 9 或第 15 行
	if !strings.Contains(res.Content, "  10| line") {
		t.Error("missing line 10")
	}
	if !strings.Contains(res.Content, "  14| line") {
		t.Error("missing line 14")
	}
	if strings.Contains(res.Content, "   9| line") {
		t.Error("should not contain line 9")
	}
	if strings.Contains(res.Content, "  15| line") {
		t.Error("should not contain line 15")
	}
}

func TestReadFile_DefaultLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "small.txt")
	writeFile(t, p, "a\nb\nc\n")
	tool := &readFileTool{}
	out, err := tool.Execute(mustJSON(t, map[string]any{"path": p}))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Lines      int `json:"lines"`
		TotalLines int `json:"total_lines"`
	}
	json.Unmarshal([]byte(out), &res)
	// "a\nb\nc\n" Split 得 4 元素（含尾空行）
	if res.Lines != 4 || res.TotalLines != 4 {
		t.Errorf("got lines=%d total=%d", res.Lines, res.TotalLines)
	}
}

func TestReadFile_OffsetBeyondEnd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tiny.txt")
	writeFile(t, p, "only\none\n")
	tool := &readFileTool{}
	out, err := tool.Execute(mustJSON(t, map[string]any{
		"path": p, "offset": 999,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Lines int `json:"lines"`
	}
	json.Unmarshal([]byte(out), &res)
	if res.Lines != 0 {
		t.Errorf("expected 0 lines when offset beyond end, got %d", res.Lines)
	}
}
