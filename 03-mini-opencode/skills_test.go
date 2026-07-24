package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillStore_LoadFromDir(t *testing.T) {
	dir := t.TempDir()

	// 创建两个技能目录
	skill1 := filepath.Join(dir, "code-review")
	os.MkdirAll(skill1, 0755)
	os.WriteFile(filepath.Join(skill1, "SKILL.md"), []byte("# Code Review\n\n审查代码质量，检查常见问题。"), 0644)

	skill2 := filepath.Join(dir, "git-workflow")
	os.MkdirAll(skill2, 0755)
	os.WriteFile(filepath.Join(skill2, "SKILL.md"), []byte("# Git Workflow\n\n规范 Git 提交流程。"), 0644)

	store := NewSkillStore(dir)
	all := store.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(all))
	}
	if all[0].Name != "code-review" {
		t.Errorf("first skill = %s, want code-review", all[0].Name)
	}
	if all[1].Name != "git-workflow" {
		t.Errorf("second skill = %s, want git-workflow", all[1].Name)
	}

	// 描述应从非标题行提取
	if all[0].Description != "审查代码质量，检查常见问题。" {
		t.Errorf("description = %q", all[0].Description)
	}
}

func TestSkillStore_EmptyDir(t *testing.T) {
	store := NewSkillStore("")
	if len(store.All()) != 0 {
		t.Error("empty dir should have 0 skills")
	}
	if store.Prompt() != "" {
		t.Error("empty store should return empty prompt")
	}
}

func TestSkillStore_NonexistentDir(t *testing.T) {
	store := NewSkillStore("/nonexistent/path")
	if len(store.All()) != 0 {
		t.Error("nonexistent dir should have 0 skills")
	}
}

func TestSkillStore_Get(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill\n\nDo stuff."), 0644)

	store := NewSkillStore(dir)
	skill, ok := store.Get("my-skill")
	if !ok {
		t.Fatal("skill not found")
	}
	if skill.Content == "" {
		t.Error("content should not be empty")
	}
	if skill.Path == "" {
		t.Error("path should not be empty")
	}

	_, ok = store.Get("nonexistent")
	if ok {
		t.Error("nonexistent skill should return false")
	}
}

func TestSkillStore_Prompt(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test\n\nA test skill."), 0644)

	store := NewSkillStore(dir)
	prompt := store.Prompt()
	if prompt == "" {
		t.Error("prompt should not be empty for non-empty store")
	}
	if !contains(prompt, "test-skill") {
		t.Errorf("prompt should contain skill name, got: %s", prompt)
	}
	if !contains(prompt, "<available_skills>") {
		t.Error("prompt should contain <available_skills> tag")
	}
}

func TestSkillStore_Install(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)

	content := "---\nname: my-skill\ndescription: A skill.\n---\n\nDo useful things."
	path, err := store.Install("my-skill", content)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("my-skill", "SKILL.md")) {
		t.Errorf("path = %s", path)
	}

	// 安装后应能通过 Get 取到，且内容一致。
	skill, ok := store.Get("my-skill")
	if !ok {
		t.Fatal("skill not found after install")
	}
	if skill.Content != content {
		t.Errorf("content mismatch: got %q", skill.Content)
	}
	// 文件确实落盘。
	if _, err := os.Stat(path); err != nil {
		t.Errorf("SKILL.md not on disk: %v", err)
	}
}

func TestSkillStore_Install_Overwrite(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)

	if _, err := store.Install("s", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install("s", "v2-content"); err != nil {
		t.Fatal(err)
	}
	skill, _ := store.Get("s")
	if skill.Content != "v2-content" {
		t.Errorf("overwrite failed: got %q", skill.Content)
	}
}

func TestSkillStore_Install_EmptyDirConfigured(t *testing.T) {
	store := NewSkillStore("")
	if _, err := store.Install("x", "y"); err == nil {
		t.Error("Install on empty dir should error")
	}
}

func TestSkillStore_Install_InvalidName(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)
	for _, bad := range []string{"", "..", ".", "a/b", `a\b`} {
		if _, err := store.Install(bad, "c"); err == nil {
			t.Errorf("Install(%q) should reject", bad)
		}
	}
}

func TestSkillStore_Install_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)
	if _, err := store.Install("s", "   "); err == nil {
		t.Error("Install with blank content should error")
	}
}

func TestSkillStore_Remove(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)

	if _, err := store.Install("gone", "# Gone\n\nbye."); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("gone"); !ok {
		t.Fatal("skill should exist before remove")
	}
	if err := store.Remove("gone"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if _, ok := store.Get("gone"); ok {
		t.Error("skill should not exist after remove")
	}
	// 目录确实被删除。
	if _, err := os.Stat(filepath.Join(dir, "gone")); !os.IsNotExist(err) {
		t.Errorf("skill dir should be removed, stat err=%v", err)
	}
}

func TestSkillStore_Remove_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)
	if err := store.Remove("nope"); err == nil {
		t.Error("Remove nonexistent skill should error")
	}
}

func TestSkillStore_Remove_InvalidName(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)
	if err := store.Remove("../escape"); err == nil {
		t.Error("Remove with traversal name should error")
	}
}

func TestInstallSkillTool_Execute(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)
	tool := &installSkillTool{store: store}

	args := `{"name":"via-tool","content":"# Via Tool\n\ninstalled by LLM."}`
	out, err := tool.Execute(args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !contains(out, `"success":true`) {
		t.Errorf("unexpected output: %s", out)
	}
	if _, ok := store.Get("via-tool"); !ok {
		t.Error("skill not registered in store after tool install")
	}
}

func TestRemoveSkillTool_Execute(t *testing.T) {
	dir := t.TempDir()
	store := NewSkillStore(dir)
	if _, err := store.Install("doomed", "# x\n\ny"); err != nil {
		t.Fatal(err)
	}
	tool := &removeSkillTool{store: store}
	out, err := tool.Execute(`{"name":"doomed"}`)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !contains(out, `"success":true`) {
		t.Errorf("unexpected output: %s", out)
	}
	if _, ok := store.Get("doomed"); ok {
		t.Error("skill still present after remove tool")
	}
}

func TestInstallSkillTool_SchemaRequired(t *testing.T) {
	tool := &installSkillTool{}
	schema := tool.Schema()
	params := schema.Function.Parameters.(map[string]any)
	props := params["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Error("schema missing name property")
	}
	if _, ok := props["content"]; !ok {
		t.Error("schema missing content property")
	}
}
