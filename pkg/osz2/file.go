package osz2

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"time"
)

// File is a read-only handle to an osz2 package entry.
// It includes the Entry metadata on top of file operations.
type File struct {
	reader    *Reader
	entry     *Entry
	mu        sync.Mutex
	offset    int64
	dirOffset int
	closed    bool
}

// Read implements fs.File.
func (f *File) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.checkReadable("read"); err != nil {
		return 0, err
	}
	n, err := f.readAt(p, f.offset)
	f.offset += int64(n)
	return n, err
}

// ReadAt implements io.ReaderAt.
func (f *File) ReadAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.checkReadable("readat"); err != nil {
		return 0, err
	}
	return f.readAt(p, offset)
}

// Seek implements io.Seeker.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return 0, f.pathError("seek", fs.ErrClosed)
	}
	if f.entry.isDir {
		return 0, f.pathError("seek", fmt.Errorf("%w: is a directory", fs.ErrInvalid))
	}

	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = f.offset
	case io.SeekEnd:
		base = f.entry.size
	default:
		return 0, f.pathError("seek", fs.ErrInvalid)
	}

	next := base + offset
	if next < 0 {
		return 0, f.pathError("seek", fs.ErrInvalid)
	}
	f.offset = next
	return next, nil
}

// ReadDir implements fs.ReadDirFile.
func (f *File) ReadDir(n int) ([]fs.DirEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil, f.pathError("readdir", fs.ErrClosed)
	}
	if !f.entry.isDir {
		return nil, f.pathError("readdir", fmt.Errorf("%w: not a directory", fs.ErrInvalid))
	}

	children := f.reader.children[f.entry.path]
	if f.dirOffset >= len(children) {
		if n > 0 {
			return nil, io.EOF
		}
		return []fs.DirEntry{}, nil
	}

	end := len(children)
	if n > 0 {
		end = min(end, f.dirOffset+n)
	}
	result := make([]fs.DirEntry, end-f.dirOffset)
	for i, entry := range children[f.dirOffset:end] {
		result[i] = entry
	}
	f.dirOffset = end
	return result, nil
}

// Close "closes" this handle.
// It doesn't close the Reader.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closed = true
	return nil
}

// Stat implements fs.File.
func (f *File) Stat() (fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil, f.pathError("stat", fs.ErrClosed)
	}
	return f.entry, nil
}

// Entry returns the metadata descriptor behind this handle.
func (f *File) Entry() *Entry { return f.entry }

// Path returns the complete package path.
func (f *File) Path() string { return f.entry.Path() }

// Name implements fs.FileInfo and fs.DirEntry.
func (f *File) Name() string { return f.entry.Name() }

// Size implements fs.FileInfo.
func (f *File) Size() int64 { return f.entry.Size() }

// Mode implements fs.FileInfo.
func (f *File) Mode() fs.FileMode { return f.entry.Mode() }

// ModTime implements fs.FileInfo.
func (f *File) ModTime() time.Time { return f.entry.ModTime() }

// IsDir implements fs.FileInfo and fs.DirEntry.
func (f *File) IsDir() bool { return f.entry.IsDir() }

// Sys implements fs.FileInfo.
func (f *File) Sys() any { return f.entry.Sys() }

// Type implements fs.DirEntry.
func (f *File) Type() fs.FileMode { return f.entry.Type() }

// Info implements fs.DirEntry.
func (f *File) Info() (fs.FileInfo, error) { return f, nil }

// CreatedAt returns the entry creation timestamp.
func (f *File) CreatedAt() time.Time { return f.entry.CreatedAt() }

// Hash returns the 16-byte file hash stored in the package.
func (f *File) Hash() [md5.Size]byte { return f.entry.Hash() }

// BeatmapID returns the assigned beatmap ID.
func (f *File) BeatmapID() (int32, bool) { return f.entry.BeatmapID() }

// Extension returns the lowercase extension without a leading dot.
func (f *File) Extension() string { return f.entry.Extension() }

// IsBeatmap reports whether the entry is an .osu beatmap.
func (f *File) IsBeatmap() bool { return f.entry.IsBeatmap() }

// IsCombinedBeatmap reports whether the entry is an .osc combined beatmap.
func (f *File) IsCombinedBeatmap() bool { return f.entry.IsCombinedBeatmap() }

// IsVideo reports whether the entry is treated as a video file.
func (f *File) IsVideo() bool { return f.entry.IsVideo() }

func (f *File) pathError(op string, err error) error {
	return &fs.PathError{Op: op, Path: f.entry.path, Err: err}
}

func (f *File) checkReadable(op string) error {
	if f.closed {
		return f.pathError(op, fs.ErrClosed)
	}
	if f.entry.isDir {
		return f.pathError(op, fmt.Errorf("%w: is a directory", fs.ErrInvalid))
	}
	return nil
}
