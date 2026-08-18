package main

import (
	"encoding/binary"
	"fmt"
)

func dibToBMP(dib []byte) ([]byte, error) {
	if len(dib) < 40 {
		return nil, fmt.Errorf("DIB header is too short")
	}

	headerSize := binary.LittleEndian.Uint32(dib[0:4])
	if headerSize != 40 && headerSize != 108 && headerSize != 124 {
		return nil, fmt.Errorf("unsupported DIB header size: %d", headerSize)
	}
	if uint64(headerSize) > uint64(len(dib)) {
		return nil, fmt.Errorf("invalid DIB header size: %d", headerSize)
	}

	bitsPerPixel := binary.LittleEndian.Uint16(dib[14:16])
	compression := binary.LittleEndian.Uint32(dib[16:20])
	colorCount := binary.LittleEndian.Uint32(dib[32:36])
	if colorCount == 0 && bitsPerPixel <= 8 {
		colorCount = 1 << bitsPerPixel
	}

	pixelOffset := uint64(headerSize) + uint64(colorCount)*4
	if headerSize == 40 && (compression == 3 || compression == 6) {
		maskSize := uint64(12)
		if compression == 6 {
			maskSize = 16
		}
		pixelOffset += maskSize
	}
	if pixelOffset > uint64(len(dib)) {
		return nil, fmt.Errorf("invalid DIB pixel offset")
	}

	const bitmapFileHeaderSize = 14
	totalSize := uint64(bitmapFileHeaderSize) + uint64(len(dib))
	if totalSize > uint64(^uint32(0)) {
		return nil, fmt.Errorf("clipboard image is too large")
	}

	header := make([]byte, bitmapFileHeaderSize)
	header[0], header[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(header[2:6], uint32(totalSize))
	binary.LittleEndian.PutUint32(header[10:14], uint32(bitmapFileHeaderSize+pixelOffset))
	return append(header, dib...), nil
}
