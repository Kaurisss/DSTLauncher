//go:build windows

package hoststats

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
	diskFreeSpace      = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// MEMORYSTATUSEX consists of two DWORDs followed by seven DWORDLONGs.
type memoryStatusEx struct {
	Length, MemoryLoad                                 uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile uint64
	TotalVirtual, AvailVirtual, AvailExtendedVirtual   uint64
}

func readMemory() (Capacity, error) {
	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	if ok, _, err := globalMemoryStatus.Call(uintptr(unsafe.Pointer(&status))); ok == 0 {
		return Capacity{}, err
	}
	return Capacity{TotalBytes: status.TotalPhys, AvailableBytes: status.AvailPhys}, nil
}

func readDisk(dir string) (Capacity, error) {
	// A trailing separator is required for UNC share roots.
	if !strings.HasSuffix(dir, `\`) {
		dir += `\`
	}
	path, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return Capacity{}, err
	}
	var available, total, free uint64
	if ok, _, err := diskFreeSpace.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&available)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	); ok == 0 {
		return Capacity{}, err
	}
	return Capacity{TotalBytes: total, AvailableBytes: available}, nil
}
