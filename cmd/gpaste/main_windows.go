//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gzj/gclip/internal/winclip"
)

func main() {
	filePaths, err := winclip.FileDropList()
	if err != nil || len(filePaths) == 0 {
		fmt.Println("Clipboard does not contain a list of files.")
		os.Exit(1)
	}

	destFolders := os.Args[1:]
	if len(destFolders) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println("Failed to get current directory:", err)
			os.Exit(1)
		}
		destFolders = []string{cwd}
	}

	var result []string
	for _, destFolder := range destFolders {
		info, err := os.Stat(destFolder)
		if err != nil || !info.IsDir() {
			fmt.Printf("Destination folder does not exist: %s\n", destFolder)
			continue
		}

		for _, sourcePath := range filePaths {
			sourcePath = filepath.Clean(sourcePath)
			info, err := os.Stat(sourcePath)
			if err != nil {
				fmt.Printf("Invalid source path: %s\n", sourcePath)
				continue
			}

			destPath, err := filepath.Abs(filepath.Join(destFolder, filepath.Base(sourcePath)))
			if err != nil {
				fmt.Printf("Failed to resolve destination path: %s\n", err)
				continue
			}
			fmt.Printf("Copying: %s -> %s\n", sourcePath, destPath)

			if info.IsDir() {
				if err := validateDirectoryDestination(sourcePath, destPath); err != nil {
					fmt.Printf("Failed to copy directory: %s\n", err)
					continue
				}
				copyDirectory(sourcePath, destPath, &result)
			} else {
				if err := copyFile(sourcePath, destPath); err != nil {
					fmt.Printf("Failed to copy file: %s\n", err)
					continue
				}
				result = append(result, fmt.Sprintf(`"%s","%s"`, sourcePath, destPath))
			}
		}
	}

	fmt.Println("\nsrc,dst")
	for _, line := range result {
		fmt.Println(line)
	}
}
