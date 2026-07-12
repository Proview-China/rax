package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/cachefacts"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
)

func main() {
	output := flag.String("output", "", "CSV output path")
	flag.Parse()
	if *output == "" {
		fail(fmt.Errorf("-output is required"))
	}
	routeCatalog, err := catalog.NewDefault(time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC))
	if err != nil {
		fail(err)
	}
	matrix, err := cachefacts.Build(routeCatalog)
	if err != nil {
		fail(err)
	}
	data, err := matrix.CSV()
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
