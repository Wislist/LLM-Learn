package main

import (
	"encoding/json"
	"fmt"

	"github.com/wislist/llmg"
)

// listSkillsTool 让 LLM 列出所有可用技能。
type listSkillsTool struct {
	store *SkillStore
}

func (t *listSkillsTool) Name() string { return "list_skills" }

func (t *listSkillsTool) Description() string {
	return "列出所有可用的技能（SKILL.md）。返回名称和简短描述。"
}

func (t *listSkillsTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "list_skills",
			Description: t.Description(),
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (t *listSkillsTool) Execute(args string) (string, error) {
	if t.store == nil {
		return `{"skills": [], "count": 0}`, nil
	}
	skills := t.store.All()
	if len(skills) == 0 {
		return `{"skills": [], "count": 0}`, nil
	}
	var entries []map[string]any
	for _, s := range skills {
		entries = append(entries, map[string]any{
			"name":        s.Name,
			"description": s.Description,
		})
	}
	out, _ := json.Marshal(map[string]any{
		"skills": entries,
		"count":  len(entries),
	})
	return string(out), nil
}

// readSkillTool 让 LLM 加载某个技能的完整指令。
type readSkillTool struct {
	store *SkillStore
}

func (t *readSkillTool) Name() string { return "read_skill" }

func (t *readSkillTool) Description() string {
	return "加载指定技能的完整 SKILL.md 内容。加载后按技能指令行事。"
}

func (t *readSkillTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "read_skill",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "技能名称（用 list_skills 查看）"},
				},
				"required": []string{"name"},
			},
		},
	}
}

func (t *readSkillTool) Execute(args string) (string, error) {
	var p struct{ Name string }
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("read_skill: invalid args: %w", err)
	}
	if t.store == nil {
		return "", fmt.Errorf("read_skill: skill store unavailable")
	}
	skill, ok := t.store.Get(p.Name)
	if !ok {
		return "", fmt.Errorf("read_skill: skill %q not found", p.Name)
	}
	out, _ := json.Marshal(map[string]any{
		"name":    skill.Name,
		"content": skill.Content,
		"path":    skill.Path,
	})
	return string(out), nil
}
