// Command continuity-reference exposes the Continuity owner-local read surface
// over a local SQLite metadata store. It is a developer-preview executable, not
// the Praxis root CLI and not a production composition root.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/cli"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/sdk"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, input io.Reader, output, errors io.Writer) int {
	flags := flag.NewFlagSet("continuity-reference", flag.ContinueOnError)
	flags.SetOutput(errors)
	database := flags.String("db", "", "path to the local Continuity SQLite metadata database")
	cursorTTL := flags.Duration("cursor-ttl", 15*time.Minute, "lifetime of emitted timeline cursors")
	flags.Usage = func() {
		fmt.Fprintln(errors, "usage: continuity-reference -db PATH {timeline show|timeline watch|checkpoint inspect}")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *database == "" || flags.NArg() == 0 {
		flags.Usage()
		return 2
	}

	store, err := sqlite.Open(ctx, *database)
	if err != nil {
		fmt.Fprintf(errors, "continuity-reference: open database: %v\n", err)
		return 1
	}
	defer store.Close()

	timeline, err := domain.NewReferenceTimeline(store, domain.SystemClock{}, *cursorTTL)
	if err != nil {
		fmt.Fprintf(errors, "continuity-reference: compose timeline reader: %v\n", err)
		return 1
	}
	client, err := sdk.New(sdk.Config{
		Timeline:    timeline,
		Checkpoints: store,
		Clock:       time.Now,
	})
	if err != nil {
		fmt.Fprintf(errors, "continuity-reference: compose SDK: %v\n", err)
		return 1
	}
	runner, err := cli.NewReadOnlyRunnerV1(client, client)
	if err != nil {
		fmt.Fprintf(errors, "continuity-reference: compose runner: %v\n", err)
		return 1
	}
	if err := runner.RunV1(ctx, flags.Args(), input, output); err != nil {
		fmt.Fprintf(errors, "continuity-reference: %v\n", err)
		return 1
	}
	return 0
}
