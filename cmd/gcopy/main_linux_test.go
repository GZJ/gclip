//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--gclip-serve" {
		runClipboardChild()
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--gclip-copy-test" {
		if err := copyToX11Clipboard(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("ready")
		return
	}
	os.Exit(m.Run())
}

func TestStartupFailure(t *testing.T) {
	t.Setenv("DISPLAY", ":65535")
	err := copyToX11Clipboard("file:///tmp/test")
	if err == nil || !strings.Contains(err.Error(), "cannot connect") {
		t.Fatalf("expected connection failure, got %v", err)
	}
}

func TestStartupTimeout(t *testing.T) {
	script := filepath.Join(t.TempDir(), "stalled-owner")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err := startClipboardChild(script, "test", 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("startup timeout did not clean up promptly")
	}
}

// Changes the current clipboard. The helper's parent exits before we read it.
func TestX11BackgroundClipboard(t *testing.T) {
	if os.Getenv("GCLIP_X11_TEST") != "1" {
		t.Skip("set GCLIP_X11_TEST=1 for desktop integration test")
	}
	c, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	atom := func(name string) xproto.Atom {
		r, e := xproto.InternAtom(c, false, uint16(len(name)), name).Reply()
		if e != nil {
			t.Fatal(e)
		}
		return r.Atom
	}
	clipboard, uri, prop := atom("CLIPBOARD"), atom("text/uri-list"), atom("GCLIP_TEST_READ")
	screen := xproto.Setup(c).DefaultScreen(c)
	w, err := xproto.NewWindowId(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = xproto.CreateWindowChecked(c, screen.RootDepth, w, screen.Root, 0, 0, 1, 1, 0, xproto.WindowClassInputOutput, screen.RootVisual, 0, nil).Check(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { xproto.SetSelectionOwner(c, w, clipboard, xproto.TimeCurrentTime); c.Sync() })
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var previous xproto.Window
	for _, content := range []string{"file:///tmp/gcopy-first", "file:///tmp/gcopy-second"} {
		cmd := exec.Command(executable, "--gclip-copy-test", content)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("copy: %v: %s", err, output)
		}
		if string(output) != "ready\n" {
			t.Fatalf("unexpected parent output %q", output)
		}
		owner, err := xproto.GetSelectionOwner(c, clipboard).Reply()
		if err != nil {
			t.Fatal(err)
		}
		if owner.Owner == 0 || owner.Owner == previous {
			t.Fatal("new owner not established")
		}
		if previous != 0 {
			deadline := time.Now().Add(2 * time.Second)
			for {
				_, err := xproto.GetGeometry(c, xproto.Drawable(previous)).Reply()
				if err != nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("old owner connection did not close")
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		previous = owner.Owner
		// Repeated reads must work after the foreground process has already exited.
		for i := 0; i < 2; i++ {
			xproto.ConvertSelection(c, w, clipboard, uri, prop, xproto.TimeCurrentTime)
			deadline := time.Now().Add(2 * time.Second)
			for {
				ev, e := c.PollForEvent()
				if e != nil {
					t.Fatal(e)
				}
				if notify, ok := ev.(xproto.SelectionNotifyEvent); ok && notify.Requestor == w {
					if notify.Property != prop {
						t.Fatal("URI conversion refused")
					}
					r, e := xproto.GetProperty(c, true, w, prop, uri, 0, 1024).Reply()
					if e != nil {
						t.Fatal(e)
					}
					if string(r.Value) != content {
						t.Fatalf("got %q, want %q", r.Value, content)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("clipboard read timed out")
				}
				time.Sleep(time.Millisecond)
			}
		}
	}
}
