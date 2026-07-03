package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileOptions struct {
	WorkDir         string
	InstructionData InstructionData
}

func normalizeFileOptions(options FileOptions) FileOptions {
	if options.WorkDir == "" {
		options.WorkDir, _ = os.Getwd()
	}
	options.WorkDir, _ = filepath.Abs(options.WorkDir)
	if options.InstructionData.MaxOutputLength == 0 {
		options.InstructionData = DefaultInstructionData()
	}
	return options
}

func resolveWorkspacePath(workDir string, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(workDir, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", abs)
	}
	return abs, nil
}

func isLikelyBinary(data []byte) bool {
	limit := len(data)
	if limit > 8000 {
		limit = 8000
	}
	for _, b := range data[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}
