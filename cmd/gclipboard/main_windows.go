//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gzj/gclip/internal/winclip"
)

func main() {
	formats, err := winclip.Formats()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(formats) > 0 {
		fmt.Print(strings.Join(formats, ","))
	}
}
