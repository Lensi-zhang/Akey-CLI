//go:build windows

package agent

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32DLL    = syscall.NewLazyDLL("kernel32.dll")
	moveFileExProc = kernel32DLL.NewProc("MoveFileExW")
)

const (
	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8
)

func replaceFile(oldPath, newPath string) error {
	oldPointer, err := syscall.UTF16PtrFromString(oldPath)
	if err != nil {
		return fmt.Errorf("encode temporary path: %w", err)
	}
	newPointer, err := syscall.UTF16PtrFromString(newPath)
	if err != nil {
		return fmt.Errorf("encode configuration path: %w", err)
	}
	result, _, callErr := moveFileExProc.Call(
		uintptr(unsafe.Pointer(oldPointer)),
		uintptr(unsafe.Pointer(newPointer)),
		moveFileReplaceExisting|moveFileWriteThrough,
	)
	if result == 0 {
		return fmt.Errorf("MoveFileExW: %w", callErr)
	}
	return nil
}
