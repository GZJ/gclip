//go:build linux

package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func main() {
	uriContent, err := getClipboardContent()
	if err != nil {
		fmt.Println("An exception occurred:", err.Error())
		os.Exit(1)
	}

	filePaths := parseURIList(uriContent)
	if len(filePaths) == 0 {
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
			sourcePath = strings.TrimRight(sourcePath, string(filepath.Separator))

			info, err := os.Stat(sourcePath)
			if err != nil {
				fmt.Printf("Invalid source path: %s\n", sourcePath)
				continue
			}

			itemName := filepath.Base(sourcePath)
			destPath := filepath.Join(destFolder, itemName)
			destPath, _ = filepath.Abs(destPath)

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

func getClipboardContent() (string, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return "", fmt.Errorf("cannot connect to X server: %v", err)
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Create a window to receive selection data
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return "", err
	}

	err = xproto.CreateWindowChecked(conn, screen.RootDepth, wid, screen.Root,
		0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput, screen.RootVisual,
		xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check()
	if err != nil {
		return "", err
	}
	defer xproto.DestroyWindow(conn, wid)

	// Get atoms
	clipboardAtom := internAtom(conn, "CLIPBOARD")
	uriListAtom := internAtom(conn, "text/uri-list")
	utf8Atom := internAtom(conn, "UTF8_STRING")
	targetProp := internAtom(conn, "GPASTE_TARGET")

	// Try text/uri-list first, then UTF8_STRING
	targets := []xproto.Atom{uriListAtom, utf8Atom}

	for _, target := range targets {
		// Request the selection
		xproto.ConvertSelection(conn, wid, clipboardAtom, target, targetProp, xproto.TimeCurrentTime)

		// Wait for SelectionNotify event
		content, err := waitForSelection(conn, wid, targetProp, 2*time.Second)
		if err == nil && len(content) > 0 {
			return content, nil
		}
	}

	return "", fmt.Errorf("failed to get clipboard content")
}

func waitForSelection(conn *xgb.Conn, wid xproto.Window, prop xproto.Atom, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ev, err := conn.PollForEvent()
		if err != nil {
			return "", err
		}

		if ev == nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		switch e := ev.(type) {
		case xproto.SelectionNotifyEvent:
			if e.Property == xproto.AtomNone {
				return "", fmt.Errorf("selection request refused")
			}

			// Read the property
			reply, err := xproto.GetProperty(conn, true, wid, prop, xproto.AtomAny, 0, 1<<20).Reply()
			if err != nil {
				return "", err
			}

			return string(reply.Value), nil
		}
	}

	return "", fmt.Errorf("timeout waiting for selection")
}

func parseURIList(content string) []string {
	var paths []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, "\r")

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "file://") {
			// Decode URI to path
			u, err := url.Parse(line)
			if err != nil {
				continue
			}
			path := u.Path
			// Handle URL encoding
			if decoded, err := url.PathUnescape(path); err == nil {
				path = decoded
			}
			paths = append(paths, path)
		}
	}

	return paths
}

func internAtom(conn *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return xproto.AtomNone
	}
	return reply.Atom
}
