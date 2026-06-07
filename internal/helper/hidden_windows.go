//go:build windows

package helper

import "syscall"

func pathHasHiddenAttribute(path string) bool {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return false
	}
	return attrs&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0
}
