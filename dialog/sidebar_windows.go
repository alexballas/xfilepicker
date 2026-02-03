//go:build windows

package dialog

import (
	"os"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
)

func driveMask() uint32 {
	dll, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		fyne.LogError("Error loading kernel32.dll", err)
		return 0
	}
	handle, err := syscall.GetProcAddress(dll, "GetLogicalDrives")
	if err != nil {
		fyne.LogError("Could not find GetLogicalDrives call", err)
		return 0
	}

	ret, _, err := syscall.SyscallN(uintptr(handle))
	if err != syscall.Errno(0) {
		fyne.LogError("Error calling GetLogicalDrives", err)
		return 0
	}

	return uint32(ret)
}

func listDrives() []string {
	var drives []string
	mask := driveMask()

	for i := 0; i < 26; i++ {
		if mask&1 == 1 {
			letter := string('A' + rune(i))
			drives = append(drives, letter+":")
		}
		mask >>= 1
	}

	return drives
}

func (s *sidebar) getPlaces() []favoriteItem {
	drives := listDrives()
	places := make([]favoriteItem, len(drives))
	for i, drive := range drives {
		driveRoot := drive + string(os.PathSeparator)
		driveRootURI, _ := storage.ListerForURI(storage.NewFileURI(driveRoot))
		places[i] = favoriteItem{
			locName: drive,
			locIcon: theme.StorageIcon(),
			loc:     driveRootURI,
		}
	}
	return places
}
