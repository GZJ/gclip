//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	_ "golang.org/x/image/bmp"

	"github.com/gzj/gclip/internal/winclip"
)

func main() {
	filename := fmt.Sprintf("clipboard_image_%s.png", time.Now().Format("20060102_150405"))
	if len(os.Args) > 1 {
		filename = os.Args[1]
	}

	data, format, err := winclip.ImageData()
	if err != nil {
		fmt.Println("No image found in the clipboard.")
		os.Exit(1)
	}
	if format == "dib" {
		data, err = dibToBMP(data)
		if err != nil {
			fmt.Println("Failed to decode clipboard image:", err)
			os.Exit(1)
		}
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		fmt.Println("Failed to decode image:", err)
		os.Exit(1)
	}

	outFile, err := os.Create(filename)
	if err != nil {
		fmt.Println("Failed to create file:", err)
		os.Exit(1)
	}
	if err := png.Encode(outFile, img); err != nil {
		outFile.Close()
		fmt.Println("Failed to save image:", err)
		os.Exit(1)
	}
	if err := outFile.Close(); err != nil {
		fmt.Println("Failed to save image:", err)
		os.Exit(1)
	}

	fullPath, err := filepath.Abs(filename)
	if err != nil {
		fmt.Println("Failed to resolve output path:", err)
		os.Exit(1)
	}
	fmt.Println(fullPath)
}
