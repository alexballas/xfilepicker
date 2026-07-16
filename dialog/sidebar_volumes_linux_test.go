//go:build linux

package dialog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxExternalVolumePaths(t *testing.T) {
	mountInfo := strings.NewReader(strings.Join([]string{
		`33 2 252:1 / / rw,relatime - ext4 /dev/mapper/system rw`,
		`62 33 259:4 / /boot rw,relatime - ext4 /dev/nvme0n1p2 rw`,
		`68 33 8:17 / /media/alex/Backup rw,nosuid,nodev - ext4 /dev/sdb1 rw`,
		`69 33 8:33 / /run/media/alex/My\040Disk rw,nosuid,nodev - exfat /dev/sdc1 rw`,
		`70 33 8:49 / /mnt/archive rw,relatime - btrfs /dev/sdd1 rw`,
		`malformed`,
	}, "\n"))

	got := linuxExternalVolumePaths(mountInfo)
	want := []string{
		"/media/alex/Backup",
		"/run/media/alex/My Disk",
		"/mnt/archive",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected external volumes: got %q, want %q", got, want)
	}
}

func TestVolumePlacesSortsDeduplicatesAndSkipsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "Alpha")
	zulu := filepath.Join(root, "Zulu")
	if err := os.MkdirAll(alpha, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(zulu, 0o755); err != nil {
		t.Fatal(err)
	}

	places := volumePlaces([]string{zulu, alpha, zulu, filepath.Join(root, "Missing")})
	if len(places) != 2 {
		t.Fatalf("expected 2 valid places, got %d", len(places))
	}
	if places[0].locName != "Alpha" || places[1].locName != "Zulu" {
		t.Fatalf("places are not sorted by path: %q, %q", places[0].locName, places[1].locName)
	}
	if places[0].loc.Path() != alpha || places[1].loc.Path() != zulu {
		t.Fatalf("unexpected place paths: %q, %q", places[0].loc.Path(), places[1].loc.Path())
	}
}
