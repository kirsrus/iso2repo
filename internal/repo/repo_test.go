package repo

import (
	"runtime"
	"testing"
)

func TestRepo_isoInPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "directory with .iso suffix found",
			path: "/mnt/data/image.iso/subdir/file.txt",
			want: "image.iso",
		},
		{
			name: "nested .iso directory returns first match",
			path: "/a/b/c.iso/d/e.iso/f.txt",
			want: "c.iso",
		},
		{
			name: "file has .iso suffix but directory does not",
			path: "/mnt/data/archive/file.iso",
			want: "",
		},
		{
			name: "no .iso directory in path",
			path: "/mnt/data/archive/file.txt",
			want: "",
		},
		{
			name: "root level .iso directory",
			path: "/backup.iso/file.txt",
			want: "backup.iso",
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
	}

	if runtime.GOOS == "windows" {
		tests = append(tests, []struct {
			name string
			path string
			want string
		}{
			{
				name: "windows path with .iso directory",
				path: `C:\Users\data\image.iso\subdir\file.txt`,
				want: "image.iso",
			}, {
				name: "windows root level .iso directory",
				path: `C:\image.iso\Users\data\subdir\file.txt`,
				want: "image.iso",
			}, {
				name: "windows file has .iso suffix but directory does not",
				path: `C:\Users\data\subdir\image.iso`,
				want: "",
			},
		}...)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Repo{}
			if got := m.iso2dirInPath(tt.path); got != tt.want {
				t.Errorf("isoInPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRepo_isoDirFullPath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		isoDir string
		want   string
	}{
		{
			name:   "first .iso directory matched",
			path:   "/a/b/c.iso/d/e.iso/f.txt",
			isoDir: "c.iso",
			want:   "/a/b/c.iso",
		},
		{
			name:   "second .iso directory when first not requested",
			path:   "/a/b/c.iso/d/e.iso/f.txt",
			isoDir: "e.iso",
			want:   "/a/b/c.iso/d/e.iso",
		},
		{
			name:   "isoDir not found in path",
			path:   "/a/b/c.iso/d/e.iso/f.txt",
			isoDir: "missing.iso",
			want:   "",
		},
		{
			name:   "isoDir without .iso suffix returns empty",
			path:   "/a/b/c.iso/d/file.txt",
			isoDir: "d.iso",
			want:   "",
		},
		{
			name:   "root level .iso directory",
			path:   "/backup.iso/subdir/file.txt",
			isoDir: "backup.iso",
			want:   "/backup.iso",
		},
	}

	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			name   string
			path   string
			isoDir string
			want   string
		}{
			name:   "windows path first .iso directory",
			path:   `C:\Users\data\image.iso\subdir\file.txt`,
			isoDir: "image.iso",
			want:   `C:\Users\data\image.iso`,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Repo{}
			got := m.isoDirFullPath(tt.path, tt.isoDir)
			if got != tt.want {
				t.Errorf("isoDirFullPath(%q, %q) = %q, want %q", tt.path, tt.isoDir, got, tt.want)
			}
		})
	}
}
