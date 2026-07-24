package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

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

// installSkillTool 让 LLM 创建一个新的本地技能（写入 SKILL.md）。
// 安装后立刻可用，无需重启。
type installSkillTool struct {
	store *SkillStore
}

func (t *installSkillTool) Name() string { return "install_skill" }

func (t *installSkillTool) Description() string {
	return "安装一个新技能：在本地技能目录下创建 <name>/SKILL.md。安装后立即可被 list_skills/read_skill 使用。已存在同名技能会被覆盖。"
}

func (t *installSkillTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "install_skill",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string", "description": "技能名称（目录名，仅含字母数字和 -_）"},
					"content": map[string]any{"type": "string", "description": "SKILL.md 的完整内容，应包含 YAML front matter（name/description）和技能指令"},
				},
				"required": []string{"name", "content"},
			},
		},
	}
}

func (t *installSkillTool) Execute(args string) (string, error) {
	var p struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("install_skill: invalid args: %w", err)
	}
	if t.store == nil {
		return "", fmt.Errorf("install_skill: skill store unavailable")
	}
	path, err := t.store.Install(p.Name, p.Content)
	if err != nil {
		return "", fmt.Errorf("install_skill: %w", err)
	}
	out, _ := json.Marshal(map[string]any{
		"success": true,
		"name":    p.Name,
		"path":    path,
	})
	return string(out), nil
}

// installSkillFromGitHubTool 让 LLM 从 GitHub 仓库下载并安装一个技能。
type installSkillFromGitHubTool struct {
	store *SkillStore
}

func (t *installSkillFromGitHubTool) Name() string { return "install_skill_from_github" }

func (t *installSkillFromGitHubTool) Description() string {
	return "从 GitHub 仓库下载并安装一个技能。需要仓库 owner/repo、仓库内技能目录路径（如 skills/.curated/my-skill），可选 ref（分支/tag，默认 main）。通过 raw.githubusercontent.com 下载 SKILL.md。"
}

func (t *installSkillFromGitHubTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "install_skill_from_github",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repo":       map[string]any{"type": "string", "description": "GitHub 仓库，形如 owner/repo"},
					"skill_path": map[string]any{"type": "string", "description": "仓库内技能目录的路径，如 skills/.curated/my-skill"},
					"ref":        map[string]any{"type": "string", "description": "分支或 tag，默认 main"},
				},
				"required": []string{"repo", "skill_path"},
			},
		},
	}
}

func (t *installSkillFromGitHubTool) Execute(args string) (string, error) {
	var p struct {
		Repo      string `json:"repo"`
		SkillPath string `json:"skill_path"`
		Ref       string `json:"ref"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("install_skill_from_github: invalid args: %w", err)
	}
	if t.store == nil {
		return "", fmt.Errorf("install_skill_from_github: skill store unavailable")
	}
	path, err := t.store.InstallFromGitHub(p.Repo, p.SkillPath, p.Ref)
	if err != nil {
		return "", fmt.Errorf("install_skill_from_github: %w", err)
	}
	out, _ := json.Marshal(map[string]any{
		"success": true,
		"name":    filepath.Base(p.SkillPath),
		"path":    path,
	})
	return string(out), nil
}

// removeSkillTool 让 LLM 删除一个已安装的本地技能。
type removeSkillTool struct {
	store *SkillStore
}

func (t *removeSkillTool) Name() string { return "remove_skill" }

func (t *removeSkillTool) Description() string {
	return "删除一个已安装的本地技能（移除其目录）。不可逆，删除前应确认。"
}

func (t *removeSkillTool) Schema() llmg.Tool {
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        "remove_skill",
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "要删除的技能名称"},
				},
				"required": []string{"name"},
			},
		},
	}
}

func (t *removeSkillTool) Execute(args string) (string, error) {
	var p struct{ Name string }
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("remove_skill: invalid args: %w", err)
	}
	if t.store == nil {
		return "", fmt.Errorf("remove_skill: skill store unavailable")
	}
	if err := t.store.Remove(p.Name); err != nil {
		return "", fmt.Errorf("remove_skill: %w", err)
	}
	out, _ := json.Marshal(map[string]any{
		"success": true,
		"name":    p.Name,
	})
	return string(out), nil
}
