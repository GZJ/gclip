//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileRejectsSameFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "source.txt")
	content := []byte("must not be truncated")
	if err := os.WriteFile(source, content, 0o644); err != nil {
		t.Fatal(err)
	}

	aliases := []struct {
		name string
		path string
	}{
		{name: "same path", path: source},
		{name: "hard link", path: filepath.Join(tempDir, "hard-link.txt")},
		{name: "symbolic link", path: filepath.Join(tempDir, "symbolic-link.txt")},
	}

	if err := os.Link(source, aliases[1].path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, aliases[2].path); err != nil {
		t.Fatal(err)
	}

	for _, alias := range aliases {
		t.Run(alias.name, func(t *testing.T) {
			if err := copyFile(source, alias.path); err == nil {
				t.Fatal("copyFile() succeeded for the same file")
			}

			got, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(content) {
				t.Fatalf("source content changed: got %q, want %q", got, content)
			}
		})
	}
}

func TestCopyFileCopiesToDifferentFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "source.txt")
	destination := filepath.Join(tempDir, "destination.txt")
	content := []byte("new content")

	if err := os.WriteFile(source, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old content that is longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(source, destination); err != nil {
		t.Fatalf("copyFile() returned error: %v", err)
	}

	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("destination content = %q, want %q", got, content)
	}
}

func TestValidateDirectoryDestination(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		destination string
		wantError   bool
	}{
		{name: "same directory", destination: source, wantError: true},
		{name: "direct child", destination: filepath.Join(source, "copy"), wantError: true},
		{name: "nested child", destination: filepath.Join(source, "a", "b", "copy"), wantError: true},
		{name: "sibling", destination: filepath.Join(tempDir, "destination"), wantError: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDirectoryDestination(source, test.destination)
			if test.wantError && err == nil {
				t.Fatal("validateDirectoryDestination() succeeded, want error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("validateDirectoryDestination() returned error: %v", err)
			}
		})
	}
}

func TestValidateDirectoryDestinationResolvesSymlinks(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}

	sourceLink := filepath.Join(tempDir, "source-link")
	if err := os.Symlink(source, sourceLink); err != nil {
		t.Fatal(err)
	}

	if err := validateDirectoryDestination(source, filepath.Join(sourceLink, "copy")); err == nil {
		t.Fatal("validateDirectoryDestination() allowed a destination inside a symlinked source")
	}
}
