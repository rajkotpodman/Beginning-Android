//go:build windows

package clipboard

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard   = user32.NewProc("OpenClipboard")
	procCloseClipboard  = user32.NewProc("CloseClipboard")
	procEmptyClipboard  = user32.NewProc("EmptyClipboard")
	procGetClipboardData= user32.NewProc("GetClipboardData")
	procSetClipboardData= user32.NewProc("SetClipboardData")

	procGlobalAlloc = kernel32.NewProc("GlobalAlloc")
	procGlobalLock  = kernel32.NewProc("GlobalLock")
	procGlobalUnlock= kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

func readSystemClipboard() (string, error) {
	r1, _, _ := procOpenClipboard.Call(0)
	if r1 == 0 {
		return "", nil
	}
	defer procCloseClipboard.Call()

	hData, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if hData == 0 {
		return "", nil
	}

	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		return "", nil
	}
	defer procGlobalUnlock.Call(hData)

	text := syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(ptr))[:])
	return text, nil
}

func writeSystemClipboard(text string) error {
	r1, _, err := procOpenClipboard.Call(0)
	if r1 == 0 {
		return err
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}

	size := uintptr(len(utf16) * 2)
	hMem, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if hMem == 0 {
		return err
	}

	ptr, _, err := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return err
	}

	dest := (*[1 << 20]uint16)(unsafe.Pointer(ptr))
	copy(dest[:], utf16)

	procGlobalUnlock.Call(hMem)
	procSetClipboardData.Call(cfUnicodeText, hMem)

	return nil
}
