//go:build linux

package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gcopy <file1> <file2> <directory1> ...")
		os.Exit(1)
	}

	var uriList []string
	var validCount int

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

		fullPath = strings.TrimRight(fullPath, string(filepath.Separator))

		uri := "file://" + fullPath
		uriList = append(uriList, uri)

		itemType := "File"
		if info.IsDir() {
			itemType = "Directory"
		}
		fmt.Printf("%s added: %s\n", itemType, fullPath)
		validCount++
	}

	if validCount == 0 {
		fmt.Println("No valid files or directories to copy.")
		os.Exit(1)
	}

	uriContent := strings.Join(uriList, "\r\n")

	if err := copyToX11Clipboard(uriContent); err != nil {
		fmt.Println("An exception occurred:", err.Error())
		os.Exit(1)
	}

	fmt.Printf("\nSuccessfully copied %d item(s) to clipboard.\n", validCount)
}

func copyToX11Clipboard(content string) error {
	conn, err := xgb.NewConn()
	if err != nil {
		return fmt.Errorf("cannot connect to X server: %v", err)
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Create a window to own the selection
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return err
	}

	err = xproto.CreateWindowChecked(conn, screen.RootDepth, wid, screen.Root,
		0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput, screen.RootVisual,
		xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check()
	if err != nil {
		return err
	}

	// Get atoms
	clipboardAtom := internAtom(conn, "CLIPBOARD")
	uriListAtom := internAtom(conn, "text/uri-list")
	utf8Atom := internAtom(conn, "UTF8_STRING")
	targetsAtom := internAtom(conn, "TARGETS")
	atomAtom := xproto.Atom(xproto.AtomAtom)

	// Take ownership of CLIPBOARD selection
	xproto.SetSelectionOwner(conn, wid, clipboardAtom, xproto.TimeCurrentTime)

	// Verify we got ownership
	reply, err := xproto.GetSelectionOwner(conn, clipboardAtom).Reply()
	if err != nil || reply.Owner != wid {
		return fmt.Errorf("failed to acquire clipboard ownership")
	}

	// Handle signals to clean up
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Fork to background to keep serving clipboard requests
	done := make(chan struct{})
	go func() {
		<-sigChan
		close(done)
	}()

	// Serve clipboard requests until another app takes ownership
	data := []byte(content)
	for {
		select {
		case <-done:
			return nil
		default:
		}

		ev, err := conn.PollForEvent()
		if err != nil {
			continue
		}
		if ev == nil {
			// Check if we still own the clipboard
			reply, err := xproto.GetSelectionOwner(conn, clipboardAtom).Reply()
			if err != nil || reply.Owner != wid {
				// Another app took ownership, we can exit
				return nil
			}
			continue
		}

		switch e := ev.(type) {
		case xproto.SelectionRequestEvent:
			response := xproto.SelectionNotifyEvent{
				Time:      e.Time,
				Requestor: e.Requestor,
				Selection: e.Selection,
				Target:    e.Target,
				Property:  e.Property,
			}

			if e.Target == targetsAtom {
				// Report supported targets
				targets := []xproto.Atom{targetsAtom, uriListAtom, utf8Atom}
				data := make([]byte, len(targets)*4)
				for i, t := range targets {
					data[i*4] = byte(t)
					data[i*4+1] = byte(t >> 8)
					data[i*4+2] = byte(t >> 16)
					data[i*4+3] = byte(t >> 24)
				}
				xproto.ChangeProperty(conn, xproto.PropModeReplace, e.Requestor,
					e.Property, atomAtom, 32, uint32(len(targets)), data)
			} else if e.Target == uriListAtom || e.Target == utf8Atom {
				xproto.ChangeProperty(conn, xproto.PropModeReplace, e.Requestor,
					e.Property, e.Target, 8, uint32(len(data)), data)
			} else {
				response.Property = xproto.AtomNone
			}

			xproto.SendEvent(conn, false, e.Requestor, 0, string(response.Bytes()))
		case xproto.SelectionClearEvent:
			// Lost ownership
			return nil
		}
	}
}

func internAtom(conn *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return xproto.AtomNone
	}
	return reply.Atom
}
