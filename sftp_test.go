package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingCloser struct {
	closed bool
}

func (c *recordingCloser) Close() error {
	c.closed = true
	return nil
}

func TestLocalTransferSizeCountsMultiplePathsRecursively(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "directory")
	nested := filepath.Join(directory, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		filepath.Join(directory, "first.txt"): "abc",
		filepath.Join(nested, "second.txt"):   "12345",
		filepath.Join(root, "standalone.txt"): "wxyz",
	}
	for name, contents := range files {
		if err := os.WriteFile(name, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	count, size, err := localTransferSize([]string{directory, filepath.Join(root, "standalone.txt")})
	if err != nil {
		t.Fatalf("localTransferSize returned an error: %v", err)
	}
	if count != 3 {
		t.Fatalf("file count = %d, want 3", count)
	}
	if size != 12 {
		t.Fatalf("total size = %d, want 12", size)
	}
}

func TestLocalTransferSizeReportsMissingPath(t *testing.T) {
	if _, _, err := localTransferSize([]string{filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("localTransferSize returned no error for a missing path")
	}
}

func TestCancelSFTPTransferStopsReaderAndClosesActiveFiles(t *testing.T) {
	app := NewApp()
	control, err := app.beginTransfer("session")
	if err != nil {
		t.Fatal(err)
	}
	closer := &recordingCloser{}
	control.add(closer)

	if stopped := app.CancelSFTPTransfer("session"); !stopped {
		t.Fatal("CancelSFTPTransfer reported no active transfer")
	}
	if !closer.closed {
		t.Fatal("active file was not closed")
	}

	reader := &transferReader{reader: strings.NewReader("contents"), reporter: &transferReporter{}, control: control}
	if _, err := reader.Read(make([]byte, 8)); !errors.Is(err, errTransferCancelled) {
		t.Fatalf("reader error = %v, want %v", err, errTransferCancelled)
	}

	app.endTransfer("session", control)
	if stopped := app.CancelSFTPTransfer("session"); stopped {
		t.Fatal("completed transfer is still registered")
	}
}
