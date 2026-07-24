package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/trustmatrix"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
)

func main() {
	csvOutput := flag.String("csv", "", "CSV output path")
	markdownOutput := flag.String("markdown", "", "Markdown output path")
	flag.Parse()
	if *csvOutput == "" || *markdownOutput == "" {
		fail(fmt.Errorf("-csv and -markdown are required"))
	}
	routeCatalog, err := catalog.NewDefault(time.Date(2026, 7, 18, 2, 30, 0, 0, time.UTC))
	if err != nil {
		fail(err)
	}
	factories, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		fail(err)
	}
	matrix, err := trustmatrix.Build(routeCatalog, factories)
	if err != nil {
		fail(err)
	}
	csvData, err := matrix.CSV()
	if err != nil {
		fail(err)
	}
	markdownData, err := matrix.Markdown()
	if err != nil {
		fail(err)
	}
	write(*csvOutput, csvData)
	write(*markdownOutput, markdownData)
}

func write(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
