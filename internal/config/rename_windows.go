//go:build windows

package config

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func atomicReplace(source, destination string) error {
	sourcePointer, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPointer, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}

	const replaceExistingAndWriteThrough = 0x1 | 0x8
	result, _, callErr := moveFileEx.Call(
		uintptr(unsafe.Pointer(sourcePointer)),
		uintptr(unsafe.Pointer(destinationPointer)),
		replaceExistingAndWriteThrough,
	)
	if result == 0 {
		if callErr == nil || callErr == syscall.Errno(0) {
			return syscall.EINVAL
		}
		return callErr
	}
	return nil
}

func isLockContention(err error) bool {
	if errors.Is(err, os.ErrExist) || errors.Is(err, os.ErrPermission) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && (errno == syscall.Errno(5) || errno == syscall.Errno(32))
}
