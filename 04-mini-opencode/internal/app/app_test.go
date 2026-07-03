package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunVersionThenQuit(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("/version\n/quit\n")

	if err := Run(context.Background(), in, &out); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "mini-opencode 0.1.0") {
		t.Fatalf("output missing version: %q", got)
	}
}
