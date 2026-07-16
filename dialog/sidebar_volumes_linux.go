//go:build linux

package dialog

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func externalVolumePaths() []string {
	mountInfo, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	defer mountInfo.Close()

	return linuxExternalVolumePaths(mountInfo)
}

func linuxExternalVolumePaths(mountInfo io.Reader) []string {
	var paths []string
	scanner := bufio.NewScanner(mountInfo)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// The mount point is field 5. Everything following it may vary because
		// mountinfo has a variable number of optional fields.
		if len(fields) < 6 {
			continue
		}

		mountPoint := unescapeMountInfoPath(fields[4])
		if isLinuxExternalVolumePath(mountPoint) {
			paths = append(paths, mountPoint)
		}
	}
	return paths
}

func isLinuxExternalVolumePath(path string) bool {
	path = filepath.Clean(path)
	for _, root := range []string{"/media", "/run/media", "/mnt"} {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Paths in /proc/self/mountinfo escape whitespace and backslashes with octal
// sequences so a simple strings.Fields call remains safe.
func unescapeMountInfoPath(path string) string {
	return strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	).Replace(path)
}
