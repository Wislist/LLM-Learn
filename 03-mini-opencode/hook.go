package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Decision 是权限钩子的裁决结果。
type Decision int

const (
	DecisionAllow   Decision = iota // 直接放行
	DecisionConfirm                 // 需要用户确认 [y/n]
	DecisionDeny                    // 直接拒绝
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionConfirm:
		return "confirm"
	case DecisionDeny:
		return "deny"
	}
	return "?"
}

// PermissionHook 在工具执行前拦截，决定是否放行。
type PermissionHook interface {
	// Check 返回裁决 + 拒绝/确认原因（给用户和 LLM 看）。
	Check(toolName, args string) (Decision, string)
}

// CLIPermissionHook 是面向终端交互的权限钩子。
type CLIPermissionHook struct {
	autoApprove bool
	output      io.Writer
	input       *bufio.Reader

	// readOnlyTools 永远放行（如 read_file）。
	readOnlyTools map[string]bool

	// denyPatterns 按工具名配置正则，命中即拒绝。
	// 默认拦截一批高危命令：rm -rf /、sudo、curl|sh、git push --force 等。
	denyPatterns map[string][]*regexp.Regexp
}

func NewCLIPermissionHook(autoApprove bool) *CLIPermissionHook {
	h := &CLIPermissionHook{
		autoApprove:   autoApprove,
		output:        os.Stdout,
		input:         bufio.NewReader(os.Stdin),
		readOnlyTools: map[string]bool{"read_file": true},
		denyPatterns:  map[string][]*regexp.Regexp{},
	}
	h.registerBashDenylist()
	return h
}

func (h *CLIPermissionHook) registerBashDenylist() {
	hazard := []string{
		`sudo\b`,
		`rm\s+-[a-zA-Z]*r[a-zA-Z]*f?\s+/(?!tmp)`, // rm -rf / (但允许 /tmp)
		`rm\s+-rf\s+/?\s*$`,                      // rm -rf /  裸根
		`:\(\)\{.*\};:`,                          // fork bomb
		`curl[^|]*\|\s*(sh|bash)`,                // curl | sh
		`wget[^|]*\|\s*(sh|bash)`,
		`git\s+push\s+(-f|--force)`, // 强推
		`mkfs`,
		`dd\s+.*of=/dev/`,               // 写裸设备
		`>\s*/dev/sd`,                   // 重定向到块设备
		`chmod\s+-R\s+0?777\s+/(?!tmp)`, // 全开权限 /
	}
	for _, p := range hazard {
		re, err := regexp.Compile(p)
		if err == nil {
			h.denyPatterns["bash"] = append(h.denyPatterns["bash"], re)
		}
	}
}

// SetAutoApprove 运行时切换自动批准（/yes 命令用）。
func (h *CLIPermissionHook) SetAutoApprove(v bool) { h.autoApprove = v }
func (h *CLIPermissionHook) AutoApprove() bool     { return h.autoApprove }

func (h *CLIPermissionHook) Check(toolName, args string) (Decision, string) {
	if h.readOnlyTools[toolName] {
		return DecisionAllow, ""
	}

	if patterns, ok := h.denyPatterns[toolName]; ok {
		for _, re := range patterns {
			if re.MatchString(args) {
				return DecisionDeny, fmt.Sprintf(
					"检测到高危操作（匹配规则 %s），已拒绝。", re.String(),
				)
			}
		}
	}

	if h.autoApprove {
		return DecisionAllow, ""
	}
	return DecisionConfirm, ""
}

// Confirm 在 DecisionConfirm 时弹出 [y/n]，返回是否放行。
func (h *CLIPermissionHook) Confirm(toolName, args string) bool {
	short := args
	if len(short) > 120 {
		short = short[:120] + "..."
	}
	fmt.Fprintf(h.output, "\n  ⚠ %s 即将执行:\n  %s\n  允许? [y/N] ", toolName, short)
	line, _ := h.input.ReadString('\n')
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}
