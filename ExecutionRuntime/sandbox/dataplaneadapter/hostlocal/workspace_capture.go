package hostlocal

import (
	"fmt"
	"io"
	"os"
)

func readWorkspaceRegularNoFollowV1(root *os.Root, relative string, before os.FileInfo) ([]byte, error) {
	file, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	// WalkDir does not follow stable symlinks. os.Root additionally prevents a
	// concurrent replacement from resolving outside the configured root; exact
	// inode checks reject replacements that resolve to another in-root object.
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("workspace file %q changed before read", relative)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(opened, after) || opened.Size() != after.Size() || opened.ModTime() != after.ModTime() || int64(len(content)) != after.Size() {
		return nil, fmt.Errorf("workspace file %q changed during read", relative)
	}
	return content, nil
}
