package dialog

import (
	"path/filepath"
	"strings"
)

// repairListedUNCChildPath restores the leading double slash that Fyne's file
// repository can lose when listing children of a Windows UNC directory.
func repairListedUNCChildPath(dirPath, childPath, childName string) (string, bool) {
	dirPath = filepath.ToSlash(dirPath)
	childPath = filepath.ToSlash(childPath)

	if !strings.HasPrefix(dirPath, "//") {
		return "", false
	}
	if childName == "" || strings.HasPrefix(childPath, "//") || !strings.HasPrefix(childPath, "/") {
		return "", false
	}

	childSuffix := "/" + childName
	if !strings.HasSuffix(childPath, childSuffix) {
		return "", false
	}

	return strings.TrimRight(dirPath, "/") + childSuffix, true
}
