package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func TestDIBToBMP(t *testing.T) {
	t.Parallel()

	// A 1x1, 24-bit, bottom-up DIB containing one red pixel and row padding.
	dib := make([]byte, 44)
	binary.LittleEndian.PutUint32(dib[0:4], 40)
	binary.LittleEndian.PutUint32(dib[4:8], 1)
	binary.LittleEndian.PutUint32(dib[8:12], 1)
	binary.LittleEndian.PutUint16(dib[12:14], 1)
	binary.LittleEndian.PutUint16(dib[14:16], 24)
	binary.LittleEndian.PutUint32(dib[20:24], 4)
	copy(dib[40:], []byte{0, 0, 255, 0})

	bmpData, err := dibToBMP(dib)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint32(bmpData[10:14]); got != 54 {
		t.Fatalf("pixel offset = %d, want 54", got)
	}

	img, _, err := image.Decode(bytes.NewReader(bmpData))
	if err != nil {
		t.Fatalf("decode converted BMP: %v", err)
	}
	if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got.R != 255 || got.G != 0 || got.B != 0 {
		t.Fatalf("pixel = %#v, want red", got)
	}
}

func TestDIBToBMPRejectsInvalidHeader(t *testing.T) {
	t.Parallel()

	if _, err := dibToBMP(make([]byte, 12)); err == nil {
		t.Fatal("dibToBMP() accepted a truncated DIB")
	}
}
