//go:build windows

package diskusage

import "golang.org/x/sys/windows"

func Free(path string) (uint64, uint64, error) {
	value, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var available, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(value, &available, &total, &totalFree); err != nil {
		return 0, 0, err
	}
	return available, total, nil
}
