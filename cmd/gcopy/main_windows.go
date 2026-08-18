//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gzj/gclip/internal/winclip"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gcopy <file1> <file2> <directory1> ...")
		os.Exit(1)
	}

	var paths []string
	for _, arg := range os.Args[1:] {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Println("Path does not exist:", arg)
			continue
		}

		fullPath, err := filepath.Abs(arg)
		if err != nil {
			fmt.Println("Failed to get absolute path:", arg)
			continue
		}
		fullPath = filepath.Clean(fullPath)
		paths = append(paths, fullPath)

		itemType := "File"
		if info.IsDir() {
			itemType = "Directory"
		}
		fmt.Printf("%s added: %s\n", itemType, fullPath)
	}

	if len(paths) == 0 {
		fmt.Println("No valid files or directories to copy.")
		os.Exit(1)
	}

	if err := winclip.SetFileDropList(paths); err != nil {
		fmt.Println("An exception occurred:", err)
		os.Exit(1)
	}

	fmt.Printf("\nSuccessfully copied %d item(s) to clipboard.\n", len(paths))
}
