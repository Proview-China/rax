package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/conformance/goldenv1"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: generate <output-directory>")
	}
	directory := os.Args[1]
	if err := os.MkdirAll(directory, 0o755); err != nil {
		panic(err)
	}
	fixtures := goldenv1.BuildV1()
	for _, name := range goldenv1.NamesV1() {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, fixtures[name], 0o644); err != nil {
			panic(err)
		}
		fmt.Println(path)
	}
}
