package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
)

func main() {
	path := flag.String("file", "", "provider-matrix Markdown path")
	flag.Parse()
	if *path == "" {
		fail(fmt.Errorf("-file is required"))
	}
	current, err := os.ReadFile(*path)
	if err != nil {
		fail(err)
	}
	generated, err := catalog.RenderCurrentBindingsMarkdown(catalog.DefaultDocument(), time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		fail(err)
	}
	updated, err := catalog.ReplaceCurrentBindingsMarkdown(current, generated)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*path, updated, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) { _, _ = fmt.Fprintln(os.Stderr, err); os.Exit(1) }
