package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wislist/mini-opencode/internal/app"
)

func main() {
	if err := app.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mini-opencode: %v\n", err)
		os.Exit(1)
	}
}
