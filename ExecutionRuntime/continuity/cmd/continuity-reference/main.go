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
	"path/filepath"
	"strings"
	"syscall"
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

	databasePath, err := preparePrivateDatabase(*database)
	if err != nil {
		fmt.Fprintf(errors, "continuity-reference: unsafe database path: %v\n", err)
		return 1
	}
	store, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		fmt.Fprintf(errors, "continuity-reference: open database: %v\n", err)
		return 1
	}
	defer store.Close()
	if err := verifyPrivateDatabase(databasePath); err != nil {
		fmt.Fprintf(errors, "continuity-reference: unsafe database after open: %v\n", err)
		return 1
	}

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

func preparePrivateDatabase(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	if err := verifyDirectoryChain(filepath.Dir(absolute)); err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	switch {
	case err == nil:
		if err := verifyPrivateDatabaseInfo(info); err != nil {
			return "", err
		}
	case os.IsNotExist(err):
		file, createErr := os.OpenFile(absolute, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr != nil {
			return "", fmt.Errorf("create private database: %w", createErr)
		}
		info, statErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil {
			return "", fmt.Errorf("inspect fresh database: %w", statErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close fresh database: %w", closeErr)
		}
		if err := verifyPrivateDatabaseInfo(info); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("inspect database path: %w", err)
	}
	return absolute, nil
}

func verifyDirectoryChain(path string) error {
	var chain []string
	for {
		chain = append(chain, path)
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	protectedByPrivateAncestor := false
	for index := len(chain) - 1; index >= 0; index-- {
		path = chain[index]
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect parent directory %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent path %q is a symbolic link", path)
		}
		if !info.IsDir() {
			return fmt.Errorf("parent path %q is not a directory", path)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("cannot verify owner of parent directory %q", path)
		}
		if !protectedByPrivateAncestor && info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("parent directory %q is writable by another user without sticky protection", path)
		}
		if stat.Uid == uint32(os.Geteuid()) && info.Mode().Perm()&0o077 == 0 {
			protectedByPrivateAncestor = true
		}
	}
	return nil
}

func verifyPrivateDatabase(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect database: %w", err)
	}
	return verifyPrivateDatabaseInfo(info)
}

func verifyPrivateDatabaseInfo(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("database path is a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database path is not a regular file")
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("database must already use private mode 0600; refusing mode %#o", info.Mode().Perm())
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("database must be owned by the current effective user")
	}
	return nil
}
