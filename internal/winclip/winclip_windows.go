//go:build windows

package winclip

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	cfText             = 1
	cfBitmap           = 2
	cfDIB              = 8
	cfUnicodeText      = 13
	cfEnhMetafile      = 14
	cfHDrop            = 15
	cfDIBV5            = 17
	gmEMMoveable       = 0x0002
	gmEMZeroInit       = 0x0040
	dragQueryFileCount = 0xffffffff
)

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	shell32               = syscall.NewLazyDLL("shell32.dll")
	procOpenClipboard     = user32.NewProc("OpenClipboard")
	procCloseClipboard    = user32.NewProc("CloseClipboard")
	procEmptyClipboard    = user32.NewProc("EmptyClipboard")
	procSetClipboardData  = user32.NewProc("SetClipboardData")
	procGetClipboardData  = user32.NewProc("GetClipboardData")
	procIsFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procRegisterFormat    = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc       = kernel32.NewProc("GlobalAlloc")
	procGlobalFree        = kernel32.NewProc("GlobalFree")
	procGlobalLock        = kernel32.NewProc("GlobalLock")
	procGlobalUnlock      = kernel32.NewProc("GlobalUnlock")
	procGlobalSize        = kernel32.NewProc("GlobalSize")
	procMoveMemory        = kernel32.NewProc("RtlMoveMemory")
	procDragQueryFile     = shell32.NewProc("DragQueryFileW")
)

// SetFileDropList puts absolute file and directory paths on the clipboard as CF_HDROP.
func SetFileDropList(paths []string) error {
	if len(paths) == 0 {
		return errors.New("file drop list is empty")
	}

	var pathList []uint16
	for _, path := range paths {
		encoded, err := syscall.UTF16FromString(path)
		if err != nil {
			return fmt.Errorf("encode path %q: %w", path, err)
		}
		pathList = append(pathList, encoded...)
	}
	pathList = append(pathList, 0)

	const dropFilesHeaderSize = 20
	totalSize := dropFilesHeaderSize + len(pathList)*2
	handle, _, callErr := procGlobalAlloc.Call(gmEMMoveable|gmEMZeroInit, uintptr(totalSize))
	if handle == 0 {
		return win32Error("GlobalAlloc", callErr)
	}
	transferred := false
	defer func() {
		if !transferred {
			procGlobalFree.Call(handle)
		}
	}()

	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		return win32Error("GlobalLock", callErr)
	}

	memory := make([]byte, totalSize)
	binary.LittleEndian.PutUint32(memory[0:4], dropFilesHeaderSize)
	binary.LittleEndian.PutUint32(memory[16:20], 1) // DROPFILES.fWide
	for i, value := range pathList {
		binary.LittleEndian.PutUint16(memory[dropFilesHeaderSize+i*2:], value)
	}
	procMoveMemory.Call(pointer, uintptr(unsafe.Pointer(&memory[0])), uintptr(len(memory)))
	runtime.KeepAlive(memory)
	globalUnlock(handle)

	err := withOpenClipboard(func() error {
		result, _, callErr := procEmptyClipboard.Call()
		if result == 0 {
			return win32Error("EmptyClipboard", callErr)
		}

		result, _, callErr = procSetClipboardData.Call(cfHDrop, handle)
		if result == 0 {
			return win32Error("SetClipboardData", callErr)
		}
		transferred = true
		return nil
	})
	return err
}

// FileDropList returns the paths stored in the clipboard's CF_HDROP value.
func FileDropList() ([]string, error) {
	var paths []string
	err := withOpenClipboard(func() error {
		handle, _, callErr := procGetClipboardData.Call(cfHDrop)
		if handle == 0 {
			return win32Error("GetClipboardData(CF_HDROP)", callErr)
		}

		count, _, _ := procDragQueryFile.Call(handle, dragQueryFileCount, 0, 0)
		paths = make([]string, 0, int(count))
		for i := uintptr(0); i < count; i++ {
			length, _, _ := procDragQueryFile.Call(handle, i, 0, 0)
			buffer := make([]uint16, int(length)+1)
			if length > 0 {
				copied, _, callErr := procDragQueryFile.Call(handle, i, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
				if copied == 0 {
					return win32Error("DragQueryFileW", callErr)
				}
			}
			paths = append(paths, syscall.UTF16ToString(buffer))
		}
		return nil
	})
	return paths, err
}

// ImageData returns PNG data when the PNG clipboard format is available,
// otherwise it returns a packed DIB.
func ImageData() ([]byte, string, error) {
	pngFormat, err := registerFormat("PNG")
	if err != nil {
		return nil, "", err
	}

	var data []byte
	var kind string
	err = withOpenClipboard(func() error {
		formats := []struct {
			id   uintptr
			kind string
		}{
			{id: uintptr(pngFormat), kind: "png"},
			{id: cfDIB, kind: "dib"},
			{id: cfDIBV5, kind: "dib"},
		}

		for _, format := range formats {
			if !isFormatAvailable(format.id) {
				continue
			}
			clipboardData, err := copyGlobalClipboardData(format.id)
			if err != nil {
				return err
			}
			data = clipboardData
			kind = format.kind
			return nil
		}
		return errors.New("clipboard does not contain an image")
	})
	return data, kind, err
}

// Formats returns Windows-style names for clipboard formats used by gclipboard.
func Formats() ([]string, error) {
	htmlFormat, err := registerFormat("HTML Format")
	if err != nil {
		return nil, err
	}
	rtfFormat, err := registerFormat("Rich Text Format")
	if err != nil {
		return nil, err
	}
	stringFormat, err := registerFormat("System.String")
	if err != nil {
		return nil, err
	}

	formats := []struct {
		id   uintptr
		name string
	}{
		{id: cfBitmap, name: "CF_BITMAP"},
		{id: cfDIB, name: "CF_DIB"},
		{id: cfEnhMetafile, name: "CF_ENHMETAFILE"},
		{id: cfHDrop, name: "CF_FILEDROP"},
		{id: uintptr(htmlFormat), name: "CF_HTML"},
		{id: uintptr(rtfFormat), name: "CF_RTF"},
		{id: uintptr(stringFormat), name: "CF_STRING"},
		{id: cfText, name: "CF_TEXT"},
		{id: cfUnicodeText, name: "CF_UNICODETEXT"},
	}

	var found []string
	for _, format := range formats {
		if isFormatAvailable(format.id) {
			found = append(found, format.name)
		}
	}
	return found, nil
}

func copyGlobalClipboardData(format uintptr) ([]byte, error) {
	handle, _, callErr := procGetClipboardData.Call(format)
	if handle == 0 {
		return nil, win32Error("GetClipboardData", callErr)
	}

	size, _, callErr := procGlobalSize.Call(handle)
	if size == 0 {
		return nil, win32Error("GlobalSize", callErr)
	}

	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		return nil, win32Error("GlobalLock", callErr)
	}
	data := make([]byte, int(size))
	procMoveMemory.Call(uintptr(unsafe.Pointer(&data[0])), pointer, size)
	runtime.KeepAlive(data)
	globalUnlock(handle)
	return data, nil
}

func registerFormat(name string) (uint32, error) {
	namePointer, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	format, _, callErr := procRegisterFormat.Call(uintptr(unsafe.Pointer(namePointer)))
	if format == 0 {
		return 0, win32Error("RegisterClipboardFormatW", callErr)
	}
	return uint32(format), nil
}

func isFormatAvailable(format uintptr) bool {
	result, _, _ := procIsFormatAvailable.Call(format)
	return result != 0
}

func withOpenClipboard(operation func() error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var openErr error
	for attempt := 0; attempt < 10; attempt++ {
		result, _, callErr := procOpenClipboard.Call(0)
		if result != 0 {
			openErr = nil
			break
		}
		openErr = win32Error("OpenClipboard", callErr)
		time.Sleep(10 * time.Millisecond)
	}
	if openErr != nil {
		return openErr
	}

	defer func() {
		result, _, callErr := procCloseClipboard.Call()
		if result == 0 && err == nil {
			err = win32Error("CloseClipboard", callErr)
		}
	}()
	return operation()
}

func globalUnlock(handle uintptr) {
	// A zero return can mean success when the lock count reaches zero. Each
	// handle here has exactly one matching GlobalLock call.
	procGlobalUnlock.Call(handle)
}

func win32Error(operation string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s failed", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
