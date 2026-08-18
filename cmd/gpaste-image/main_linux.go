//go:build linux

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	_ "image/jpeg"

	_ "golang.org/x/image/bmp"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func main() {
	filename := ""
	if len(os.Args) > 1 {
		filename = os.Args[1]
	} else {
		filename = fmt.Sprintf("clipboard_image_%s.png", time.Now().Format("20060102_150405"))
	}

	imgData, err := getClipboardImage()
	if err != nil {
		fmt.Println("No image found in the clipboard.")
		os.Exit(1)
	}

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		fmt.Println("Failed to decode image:", err)
		os.Exit(1)
	}

	// Save as PNG
	outFile, err := os.Create(filename)
	if err != nil {
		fmt.Println("Failed to create file:", err)
		os.Exit(1)
	}
	defer outFile.Close()

	if err := png.Encode(outFile, img); err != nil {
		fmt.Println("Failed to save image:", err)
		os.Exit(1)
	}

	fullPath, _ := filepath.Abs(filename)
	fmt.Println(fullPath)
}

func getClipboardImage() ([]byte, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to X server: %v", err)
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
	pngAtom := internAtom(conn, "image/png")
	bmpAtom := internAtom(conn, "image/bmp")
	jpegAtom := internAtom(conn, "image/jpeg")
	targetProp := internAtom(conn, "GPASTE_IMAGE_TARGET")

	// Try different image formats in order of preference
	targets := []xproto.Atom{pngAtom, jpegAtom, bmpAtom}

	for _, target := range targets {
		xproto.ConvertSelection(conn, wid, clipboardAtom, target, targetProp, xproto.TimeCurrentTime)

		data, err := waitForSelection(conn, wid, targetProp, 2*time.Second)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}

	return nil, fmt.Errorf("no image in clipboard")
}

func waitForSelection(conn *xgb.Conn, wid xproto.Window, prop xproto.Atom, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)

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
				return nil, fmt.Errorf("selection request refused")
			}

			// Read the property in chunks for large images
			return readPropertyFully(conn, wid, prop)
		}
	}

	return nil, fmt.Errorf("timeout waiting for selection")
}

func readPropertyFully(conn *xgb.Conn, wid xproto.Window, prop xproto.Atom) ([]byte, error) {
	var data []byte
	offset := uint32(0)
	chunkSize := uint32(1 << 20) // 1MB chunks

	for {
		reply, err := xproto.GetProperty(conn, false, wid, prop, xproto.AtomAny, offset/4, chunkSize/4).Reply()
		if err != nil {
			return nil, err
		}

		data = append(data, reply.Value...)

		if reply.BytesAfter == 0 {
			break
		}
		offset += uint32(len(reply.Value))
	}

	// Delete the property
	xproto.DeleteProperty(conn, wid, prop)

	return data, nil
}

func internAtom(conn *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return xproto.AtomNone
	}
	return reply.Atom
}
