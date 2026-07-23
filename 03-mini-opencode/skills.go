package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill 表示一个本地技能，由 SKILL.md 文件定义。
type Skill struct {
	Name        string // 技能名（目录名）
	Path        string // SKILL.md 绝对路径
	Description string // 从 SKILL.md 第一段提取的描述
	Content     string // 完整 SKILL.md 内容
}

// SkillStore 管理技能的发现和加载。
type SkillStore struct {
	dir    string
	skills map[string]*Skill
}

// NewSkillStore 创建一个技能存储，从 dir 加载所有 SKILL.md。
// dir 不存在时返回空 store（不报错，技能是可选的）。
func NewSkillStore(dir string) *SkillStore {
	s := &SkillStore{
		dir:    dir,
		skills: map[string]*Skill{},
	}
	if dir == "" {
		return s
	}
	s.Reload()
	return s
}

// Reload 重新扫描目录，加载所有含 SKILL.md 的子目录。
func (s *SkillStore) Reload() {
	if s.dir == "" {
		return
	}
	s.skills = map[string]*Skill{}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(s.dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}
		content := string(data)
		skill := &Skill{
			Name:        entry.Name(),
			Path:        skillPath,
			Content:     content,
			Description: extractDescription(content),
		}
		s.skills[skill.Name] = skill
	}
}

// All 返回按名称排序的所有技能。
func (s *SkillStore) All() []*Skill {
	out := make([]*Skill, 0, len(s.skills))
	for _, skill := range s.skills {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Get 返回指定名称的技能。
func (s *SkillStore) Get(name string) (*Skill, bool) {
	skill, ok := s.skills[name]
	return skill, ok
}

// Prompt 返回技能列表的简短描述，用于注入 system prompt。
func (s *SkillStore) Prompt() string {
	skills := s.All()
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n<available_skills>\n")
	for _, skill := range skills {
		fmt.Fprintf(&sb, "- %s: %s\n", skill.Name, skill.Description)
	}
	sb.WriteString("用 list_skills 查看详情，用 read_skill 加载完整技能指令。\n")
	sb.WriteString("</available_skills>")
	return sb.String()
}

// extractDescription 从 SKILL.md 内容提取简短描述。
// 取第一段非标题、非空行的文本（截断到 120 字符）。
func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	var desc string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		desc = trimmed
		break
	}
	if len([]rune(desc)) > 120 {
		desc = string([]rune(desc)[:120]) + "..."
	}
	return desc
}
