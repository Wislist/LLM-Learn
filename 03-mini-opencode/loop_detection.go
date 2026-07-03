package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/wislist/llmg"
)

const (
	loopWindow    = 10 // 检查最近 N 个 tool call
	loopMaxRepeat = 3  // 同一签名出现超过 N 次判定为循环
)

// toolSignature 计算一个 tool call 的签名（tool 名 + 参数 hash）。
// 参数 hash 而非原文，避免长参数污染窗口。
func toolSignature(tc llmg.ToolCall) string {
	h := sha256.Sum256([]byte(tc.Function.Name + "|" + tc.Function.Arguments))
	return tc.Function.Name + ":" + hex.EncodeToString(h[:8])
}

// detectLoop 检查最近若干个 tool call 签名是否重复超阈值。
// steps 是按时间顺序的签名切片（旧的在前）。
// 返回 true 表示检测到死循环。
func detectLoop(steps []string) (bool, string) {
	if len(steps) < loopWindow {
		return false, ""
	}
	window := steps[len(steps)-loopWindow:]
	counts := make(map[string]int)
	for _, s := range window {
		counts[s]++
		if counts[s] > loopMaxRepeat {
			return true, fmt.Sprintf(
				"检测到死循环：工具调用 %s 在最近 %d 步内重复 %d 次，中止本轮。",
				strings.SplitN(s, ":", 2)[0], loopWindow, counts[s],
			)
		}
	}
	return false, ""
}
