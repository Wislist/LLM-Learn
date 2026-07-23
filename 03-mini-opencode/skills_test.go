package main

import (
	"os"
	"path/filepath"
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


