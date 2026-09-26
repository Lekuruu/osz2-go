package osz2

import (
	"crypto/md5"
	"io/fs"
	"path"
	"strings"
	"time"
)

// Entry is metadata for a file in a package.
// It does not contain any actual file contents.
type Entry struct {
	path         string
	size         int64
	dataOffset   int64
	hash         [md5.Size]byte
	createdAt    time.Time
	modifiedAt   time.Time
	beatmapID    int32
	hasBeatmapID bool
	isDir        bool
}

func newDirectoryEntry(entryPath string) *Entry {
	return &Entry{path: entryPath, isDir: true}
}

// Path returns the complete path within the package (with slashes and stuff).
func (e *Entry) Path() string {
	if e == nil {
		return ""
	}
	return e.path
}

// Name implements fs.FileInfo / fs.DirEntry.
func (e *Entry) Name() string {
	if e == nil {
		return ""
	}
	if e.path == "." {
		return "."
	}
	return path.Base(e.path)
}

// Size implements fs.FileInfo.
func (e *Entry) Size() int64 {
	if e == nil || e.isDir {
		return 0
	}
	return e.size
}

// Mode implements fs.FileInfo.
func (e *Entry) Mode() fs.FileMode {
	// The filesystem we provide is read-only, thus
	// the files here should return a read-only mode
	if e != nil && e.isDir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}

// ModTime implements fs.FileInfo.
func (e *Entry) ModTime() time.Time {
	if e == nil {
		return time.Time{}
	}
	return e.modifiedAt
}

// IsDir implements fs.FileInfo / fs.DirEntry.
func (e *Entry) IsDir() bool {
	return e != nil && e.isDir
}

// Sys implements fs.FileInfo.
func (e *Entry) Sys() any {
	return e
}

// Type implements fs.DirEntry.
func (e *Entry) Type() fs.FileMode {
	return e.Mode().Type()
}

// Info implements fs.DirEntry.
func (e *Entry) Info() (fs.FileInfo, error) {
	return e, nil
}

// CreatedAt returns the entry creation timestamp.
func (e *Entry) CreatedAt() time.Time {
	if e == nil {
		return time.Time{}
	}
	return e.createdAt
}

// Hash returns the 16-byte file hash stored in the package.
func (e *Entry) Hash() [md5.Size]byte {
	if e == nil {
		return [md5.Size]byte{}
	}
	return e.hash
}

// BeatmapID returns the assigned beatmap ID.
func (e *Entry) BeatmapID() (int32, bool) {
	if e == nil || !e.hasBeatmapID {
		return 0, false
	}
	return e.beatmapID, true
}

// Extension returns the lowercase extension without a leading dot.
func (e *Entry) Extension() string {
	if e == nil || e.isDir {
		return ""
	}
	extension := strings.ToLower(path.Ext(strings.TrimSpace(e.path)))
	return strings.TrimPrefix(extension, ".")
}

// IsBeatmap reports whether the entry is an .osu beatmap.
func (e *Entry) IsBeatmap() bool {
	return e.Extension() == "osu"
}

// IsCombinedBeatmap reports whether the entry is an .osc combined beatmap.
// https://github.com/ppy/osu-stream/blob/master/BeatmapCombinator/Program.cs#L31
func (e *Entry) IsCombinedBeatmap() bool {
	return e.Extension() == "osc"
}

// IsVideo reports whether the entry is treated as a video file.
func (e *Entry) IsVideo() bool {
	_, ok := videoFileExtensions[e.Extension()]
	return ok
}

func (e *Entry) isBeatmapFile() bool {
	return e.IsBeatmap() || e.IsCombinedBeatmap()
}
