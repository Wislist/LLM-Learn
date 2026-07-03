package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v map[string]any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEditTool_UniqueMatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	writeFile(t, p, "package main\n\nfunc foo() {}\n")
	tool := &editTool{}
	_, err := tool.Execute(mustJSON(t, map[string]any{
		"file_path": p, "old_string": "func foo() {}", "new_string": "func foo() error { return nil }",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(p)
	want := "package main\n\nfunc foo() error { return nil }\n"
	if string(got) != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEditTool_MultipleMatchNoReplaceAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "b.go")
	writeFile(t, p, "x\nx\nx\n")
	tool := &editTool{}
	_, err := tool.Execute(mustJSON(t, map[string]any{
		"file_path": p, "old_string": "x", "new_string": "y",
	}))
	if err == nil || !strings.Contains(err.Error(), "3 次") {
		t.Fatalf("want multi-match error, got: %v", err)
	}
}

func TestEditTool_ReplaceAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.go")
	writeFile(t, p, "x\nx\nx\n")
	tool := &editTool{}
	_, err := tool.Execute(mustJSON(t, map[string]any{
		"file_path": p, "old_string": "x", "new_string": "y", "replace_all": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "y\ny\ny\n" {
		t.Errorf("got %q", got)
	}
}

func TestEditTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "d.go")
	writeFile(t, p, "package main\nfunc bar() {}\n")
	tool := &editTool{}
	_, err := tool.Execute(mustJSON(t, map[string]any{
		"file_path": p, "old_string": "func baz() {}", "new_string": "func baz() error { return nil }",
	}))
	if err == nil || !strings.Contains(err.Error(), "未在文件中找到") {
		t.Fatalf("want not-found error, got: %v", err)
	}
}

func TestEditTool_SameStrings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "e.go")
	writeFile(t, p, "hello\n")
	tool := &editTool{}
	_, err := tool.Execute(mustJSON(t, map[string]any{
		"file_path": p, "old_string": "hello", "new_string": "hello",
	}))
	if err == nil || !strings.Contains(err.Error(), "相同") {
		t.Fatalf("want same-string error, got: %v", err)
	}
}
