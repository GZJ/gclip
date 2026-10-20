//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--gclip-serve" {
		runClipboardChild()
		return
	}
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

const clipboardStartupTimeout = 5 * time.Second

type clipboardStartup struct {
	Error string `json:"error,omitempty"`
}

func copyToX11Clipboard(content string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return startClipboardChild(executable, content, clipboardStartupTimeout)
}

func startClipboardChild(executable, content string, timeout time.Duration) error {
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	defer writer.Close()
	cmd := exec.Command(executable, "--gclip-serve")
	cmd.Stdin = strings.NewReader(content)
	// No terminal or output pipe is retained by the detached owner.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.ExtraFiles = []*os.File{writer}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start clipboard owner: %w", err)
	}
	writer.Close()
	ready := make(chan error, 1)
	go func() {
		var status clipboardStartup
		if err := json.NewDecoder(reader).Decode(&status); err != nil {
			ready <- fmt.Errorf("clipboard owner exited before becoming ready: %w", err)
		} else if status.Error != "" {
			ready <- fmt.Errorf("%s", status.Error)
		} else {
			ready <- nil
		}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err = <-ready:
	case <-timer.C:
		err = fmt.Errorf("clipboard owner startup timed out after %s", timeout)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	// The child now owns CLIPBOARD and exits when another application replaces it.
	return cmd.Process.Release()
}

func runClipboardChild() {
	status := os.NewFile(3, "clipboard-startup")
	if status == nil {
		os.Exit(1)
	}
	defer status.Close()
	content, err := io.ReadAll(os.Stdin)
	_ = os.Stdin.Close()
	if err == nil {
		err = serveX11Clipboard(string(content), func() error {
			if err := json.NewEncoder(status).Encode(clipboardStartup{}); err != nil {
				return err
			}
			return status.Close()
		})
	}
	if err != nil {
		// After readiness the pipe is closed; failures simply terminate the owner.
		_ = json.NewEncoder(status).Encode(clipboardStartup{Error: err.Error()})
		os.Exit(1)
	}
}

func serveX11Clipboard(content string, ready func() error) error {
	conn, err := xgb.NewConn()
	if err != nil {
		return fmt.Errorf("cannot connect to X server: %w", err)
	}
	defer conn.Close()
	// Fail before taking ownership rather than advertise data we cannot transfer.
	if len(content)+24 > int(xproto.Setup(conn).MaximumRequestLength)*4 {
		return fmt.Errorf("file URI list exceeds the X11 transfer limit")
	}

	atoms := make(map[string]xproto.Atom)
	for _, name := range []string{"CLIPBOARD", "text/uri-list", "UTF8_STRING", "TARGETS", "MULTIPLE", "ATOM_PAIR"} {
		reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
		if err != nil {
			return fmt.Errorf("intern %s: %w", name, err)
		}
		atoms[name] = reply.Atom
	}
	screen := xproto.Setup(conn).DefaultScreen(conn)
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return err
	}
	if err := xproto.CreateWindowChecked(conn, screen.RootDepth, wid, screen.Root,
		0, 0, 1, 1, 0, xproto.WindowClassInputOutput, screen.RootVisual,
		xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check(); err != nil {
		return err
	}

	xproto.SetSelectionOwner(conn, wid, atoms["CLIPBOARD"], xproto.TimeCurrentTime)
	owner, err := xproto.GetSelectionOwner(conn, atoms["CLIPBOARD"]).Reply()
	if err != nil {
		return fmt.Errorf("query clipboard ownership: %w", err)
	}
	if owner.Owner != wid {
		return fmt.Errorf("failed to acquire clipboard ownership")
	}
	if err := ready(); err != nil {
		return err
	}
	data := []byte(content)

	for {
		ev, err := conn.WaitForEvent()
		if err != nil {
			return fmt.Errorf("X11 clipboard event: %w", err)
		}
		if ev == nil {
			return fmt.Errorf("X11 clipboard connection closed")
		}
		switch e := ev.(type) {
		case xproto.SelectionRequestEvent:
			// A requestor may disappear during a transfer; keep serving other clients.
			_ = serveSelection(conn, e, atoms, data)
		case xproto.SelectionClearEvent:
			if e.Selection == atoms["CLIPBOARD"] {
				return nil
			}
		}
	}
}

func atomBytes(atoms []xproto.Atom) []byte {
	data := make([]byte, len(atoms)*4)
	for i, atom := range atoms {
		xgb.Put32(data[i*4:], uint32(atom))
	}
	return data
}

func serveSelection(conn *xgb.Conn, e xproto.SelectionRequestEvent, atoms map[string]xproto.Atom, content []byte) error {
	property := e.Property
	if property == xproto.AtomNone && e.Target != atoms["MULTIPLE"] {
		property = e.Target
	}
	response := xproto.SelectionNotifyEvent{Time: e.Time, Requestor: e.Requestor, Selection: e.Selection, Target: e.Target, Property: property}
	writeTarget := func(target, prop xproto.Atom) bool {
		if prop == xproto.AtomNone {
			return false
		}
		var data []byte
		typ, format := target, byte(8)
		switch target {
		case atoms["TARGETS"]:
			data = atomBytes([]xproto.Atom{atoms["TARGETS"], atoms["MULTIPLE"], atoms["text/uri-list"], atoms["UTF8_STRING"]})
			typ, format = xproto.AtomAtom, 32
		case atoms["text/uri-list"], atoms["UTF8_STRING"]:
			data = content
		default:
			return false
		}
		// Large selections need INCR, which this command does not implement.
		if len(data)+24 > int(xproto.Setup(conn).MaximumRequestLength)*4 {
			return false
		}
		return xproto.ChangePropertyChecked(conn, xproto.PropModeReplace, e.Requestor, prop,
			typ, format, uint32(len(data)/(int(format)/8)), data).Check() == nil
	}
	if e.Selection != atoms["CLIPBOARD"] {
		response.Property = xproto.AtomNone
	} else if e.Target == atoms["MULTIPLE"] && property != xproto.AtomNone {
		pairs, err := xproto.GetProperty(conn, false, e.Requestor, property, atoms["ATOM_PAIR"], 0, 4096).Reply()
		if err != nil {
			return fmt.Errorf("read MULTIPLE request: %w", err)
		}
		if pairs.Type != atoms["ATOM_PAIR"] || pairs.Format != 32 || pairs.BytesAfter != 0 || len(pairs.Value)%8 != 0 {
			response.Property = xproto.AtomNone
		} else {
			for i := 0; i < len(pairs.Value); i += 8 {
				target := xproto.Atom(xgb.Get32(pairs.Value[i:]))
				prop := xproto.Atom(xgb.Get32(pairs.Value[i+4:]))
				if prop == property || !writeTarget(target, prop) {
					xgb.Put32(pairs.Value[i+4:], 0)
				}
			}
			if err := xproto.ChangePropertyChecked(conn, xproto.PropModeReplace, e.Requestor,
				property, atoms["ATOM_PAIR"], 32, uint32(len(pairs.Value)/4), pairs.Value).Check(); err != nil {
				return err
			}
		}
	} else if !writeTarget(e.Target, property) {
		response.Property = xproto.AtomNone
	}
	return xproto.SendEventChecked(conn, false, e.Requestor, 0, string(response.Bytes())).Check()
}
