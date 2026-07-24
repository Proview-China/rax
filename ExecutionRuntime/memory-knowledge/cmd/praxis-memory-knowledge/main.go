package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, cli.HTTPTransport{BaseURL: os.Getenv("PRAXIS_MEMORY_KNOWLEDGE_API")}); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
