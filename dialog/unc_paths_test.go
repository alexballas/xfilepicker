package dialog

import "testing"

func TestRepairListedUNCChildPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dirPath   string
		childPath string
		childName string
		want      string
		ok        bool
	}{
		{
			name:      "repair missing leading slash for file",
			dirPath:   "//192.168.88.100/share/media",
			childPath: "/192.168.88.100/share/media/video.mp4",
			childName: "video.mp4",
			want:      "//192.168.88.100/share/media/video.mp4",
			ok:        true,
		},
		{
			name:      "repair missing leading slash for dir",
			dirPath:   "//192.168.88.100/share/media",
			childPath: "/192.168.88.100/share/media/season1",
			childName: "season1",
			want:      "//192.168.88.100/share/media/season1",
			ok:        true,
		},
		{
			name:      "keep already valid unc path",
			dirPath:   "//192.168.88.100/share/media",
			childPath: "//192.168.88.100/share/media/video.mp4",
			childName: "video.mp4",
			want:      "",
			ok:        false,
		},
		{
			name:      "ignore non unc dir",
			dirPath:   "C:/media",
			childPath: "/media/video.mp4",
			childName: "video.mp4",
			want:      "",
			ok:        false,
		},
		{
			name:      "ignore suffix mismatch",
			dirPath:   "//192.168.88.100/share/media",
			childPath: "/192.168.88.100/share/media/other.mp4",
			childName: "video.mp4",
			want:      "",
			ok:        false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := repairListedUNCChildPath(tc.dirPath, tc.childPath, tc.childName)
			if ok != tc.ok {
				t.Fatalf("repairListedUNCChildPath() ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("repairListedUNCChildPath() = %q, want %q", got, tc.want)
			}
		})
	}
}
