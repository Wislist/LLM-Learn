package main

import (
	"fmt"
	"io"
	"net/http"
)

// fetchGitHubRaw 下载一个 GitHub raw 文件 URL 并返回其内容。
// 超时与重定向由 http.Client 默认行为处理。
func fetchGitHubRaw(rawURL string) (string, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
