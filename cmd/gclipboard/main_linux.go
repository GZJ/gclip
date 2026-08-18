//go:build linux

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func main() {
	targets, err := getClipboardTargets()
	if err != nil {
		return
	}

	// Map X11 targets to Windows-like format names
	formatMap := map[string]string{
		"image/png":     "CF_BITMAP",
		"image/bmp":     "CF_BITMAP",
		"image/jpeg":    "CF_BITMAP",
		"image/x-bmp":   "CF_DIB",
		"text/uri-list": "CF_FILEDROP",
		"text/html":     "CF_HTML",
		"text/richtext": "CF_RTF",
		"text/plain":    "CF_TEXT",
		"UTF8_STRING":   "CF_UNICODETEXT",
		"STRING":        "CF_STRING",
		"TEXT":          "CF_TEXT",
	}

	foundFormats := make(map[string]bool)

	for _, target := range targets {
		if format, ok := formatMap[target]; ok {
			foundFormats[format] = true
		}
	}

	var formats []string
	for format := range foundFormats {
		formats = append(formats, format)
	}

	if len(formats) > 0 {
		fmt.Print(strings.Join(formats, ","))
	}
}

func getClipboardTargets() ([]string, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Create a window to receive selection data
	wid, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}

	err = xproto.CreateWindowChecked(conn, screen.RootDepth, wid, screen.Root,
		0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput, screen.RootVisual,
		xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check()
	if err != nil {
		return nil, err
	}
	defer xproto.DestroyWindow(conn, wid)

	// Get atoms
	clipboardAtom := internAtom(conn, "CLIPBOARD")
	targetsAtom := internAtom(conn, "TARGETS")
	targetProp := internAtom(conn, "GCLIPBOARD_TARGET")

	// Request TARGETS
	xproto.ConvertSelection(conn, wid, clipboardAtom, targetsAtom, targetProp, xproto.TimeCurrentTime)

	// Wait for response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ev, err := conn.PollForEvent()
		if err != nil {
			return nil, err
		}

		if ev == nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		switch e := ev.(type) {
		case xproto.SelectionNotifyEvent:
			if e.Property == xproto.AtomNone {
				return nil, fmt.Errorf("no clipboard content")
			}

			// Read the TARGETS property
			reply, err := xproto.GetProperty(conn, true, wid, targetProp, xproto.AtomAtom, 0, 1<<16).Reply()
			if err != nil {
				return nil, err
			}

			// Parse atoms from the response
			atoms := parseAtoms(reply.Value)
			return atomsToNames(conn, atoms), nil
		}
	}

	return nil, fmt.Errorf("timeout")
}

func parseAtoms(data []byte) []xproto.Atom {
	var atoms []xproto.Atom
	for i := 0; i+4 <= len(data); i += 4 {
		atom := xproto.Atom(uint32(data[i]) | uint32(data[i+1])<<8 | uint32(data[i+2])<<16 | uint32(data[i+3])<<24)
		atoms = append(atoms, atom)
	}
	return atoms
}

func atomsToNames(conn *xgb.Conn, atoms []xproto.Atom) []string {
	var names []string
	for _, atom := range atoms {
		if atom == xproto.AtomNone {
			continue
		}
		reply, err := xproto.GetAtomName(conn, atom).Reply()
		if err != nil {
			continue
		}
		names = append(names, reply.Name)
	}
	return names
}

func internAtom(conn *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return xproto.AtomNone
	}
	return reply.Atom
}
