package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Open without O_TRUNC so a source that is also reachable through dst is
	// detected before any data is destroyed.
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	dstInfo, err := dstFile.Stat()
	if err != nil {
		return err
	}
	if os.SameFile(srcInfo, dstInfo) {
		return fmt.Errorf("source and destination are the same file: %s", src)
	}

	if err := dstFile.Truncate(0); err != nil {
		return err
	}
	if _, err := dstFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func validateDirectoryDestination(srcDir, dstDir string) error {
	resolvedSource, err := resolvePath(srcDir)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}

	resolvedDestination, err := resolvePath(dstDir)
	if err != nil {
		return fmt.Errorf("resolve destination directory: %w", err)
	}

	if !strings.EqualFold(filepath.VolumeName(resolvedSource), filepath.VolumeName(resolvedDestination)) {
		return nil
	}

	rel, err := filepath.Rel(resolvedSource, resolvedDestination)
	if err != nil {
		return fmt.Errorf("compare source and destination: %w", err)
	}

	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("destination must not be the source directory or one of its subdirectories")
	}

	return nil
}

// resolvePath resolves symlinks in both existing paths and the nearest
// existing ancestor of a path that has not been created yet.
func resolvePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	current := filepath.Clean(absPath)
	var missingParts []string

	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missingParts) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missingParts[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missingParts = append(missingParts, filepath.Base(current))
		current = parent
	}
}

func copyDirectory(srcDir, dstDir string, result *[]string) {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		fmt.Printf("Failed to create directory: %s\n", err)
		return
	}
	*result = append(*result, fmt.Sprintf(`"%s","%s"`, srcDir, dstDir))

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		fmt.Printf("Failed to read directory: %s\n", err)
		return
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {
			copyDirectory(srcPath, dstPath, result)
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				fmt.Printf("Failed to copy file: %s\n", err)
				continue
			}
			*result = append(*result, fmt.Sprintf(`"%s","%s"`, srcPath, dstPath))
		}
	}
}
