//go:build darwin

package dialog

import (
	"os"
	"path/filepath"
)

func externalVolumePaths() []string {
	const volumesRoot = "/Volumes"
	entries, err := os.ReadDir(volumesRoot)
	if err != nil {
		return nil
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(volumesRoot, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}
