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

// validateSkillName 确保技能名只含安全字符，防止目录穿越。
func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("skill name is empty")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("skill name %q is invalid (must be a simple directory name)", name)
	}
	return nil
}

// Install 创建或覆盖一个本地技能：在技能目录下写入 SKILL.md，然后重新加载。
// 返回写入的技能路径。
func (s *SkillStore) Install(name, content string) (string, error) {
	if s.dir == "" {
		return "", fmt.Errorf("skill store has no directory configured")
	}
	if err := validateSkillName(name); err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("skill content is empty")
	}
	skillDir := filepath.Join(s.dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", fmt.Errorf("create skill dir: %w", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}
	s.Reload()
	return skillPath, nil
}

// InstallFromGitHub 从 GitHub 仓库下载一个技能目录（含 SKILL.md）到本地技能目录。
// repo 形如 "owner/repo"；skillPath 是仓库内技能目录的路径（如 "skills/.curated/my-skill"）；
// ref 是分支或 tag（默认 "main"）。下载通过 GitHub raw 文件接口完成，无需 git。
func (s *SkillStore) InstallFromGitHub(repo, skillPath, ref string) (string, error) {
	if s.dir == "" {
		return "", fmt.Errorf("skill store has no directory configured")
	}
	if repo == "" || skillPath == "" {
		return "", fmt.Errorf("repo and skill path are required")
	}
	if ref == "" {
		ref = "main"
	}
	name := filepath.Base(skillPath)
	if err := validateSkillName(name); err != nil {
		return "", err
	}

	// 下载 SKILL.md（技能的必需文件）。
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/SKILL.md", repo, ref, skillPath)
	body, err := fetchGitHubRaw(rawURL)
	if err != nil {
		return "", fmt.Errorf("download SKILL.md from %s: %w", rawURL, err)
	}

	skillDir := filepath.Join(s.dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return "", fmt.Errorf("create skill dir: %w", err)
	}
	skillPathOnDisk := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPathOnDisk, []byte(body), 0o644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}
	s.Reload()
	return skillPathOnDisk, nil
}

// Remove 删除一个本地技能目录。不存在时返回错误。
func (s *SkillStore) Remove(name string) error {
	if s.dir == "" {
		return fmt.Errorf("skill store has no directory configured")
	}
	if err := validateSkillName(name); err != nil {
		return err
	}
	skillDir := filepath.Join(s.dir, name)
	if _, err := os.Stat(skillDir); err != nil {
		return fmt.Errorf("skill %q not found", name)
	}
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("remove skill %q: %w", name, err)
	}
	s.Reload()
	return nil
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
