//go:build windows

package metrics

import "golang.org/x/sys/windows"

// DiskAvailableBytes returns the available bytes on the filesystem containing path.
func DiskAvailableBytes(path string) (uint64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytesAvailable uint64
	err = windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, nil, nil)
	if err != nil {
		return 0, err
	}
	return freeBytesAvailable, nil
}
