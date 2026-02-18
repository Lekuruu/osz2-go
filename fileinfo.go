package osz2

import (
	"path/filepath"
	"strings"
	"time"
)

// FileInfo represents information about a file in the osz2 package
type FileInfo struct {
	FileName     string
	Offset       int32
	Size         int32
	Hash         []byte
	DateCreated  time.Time
	DateModified time.Time
}

// NewFileInfo creates a new FileInfo instance
func NewFileInfo(fileName string, offset, size int32, hash []byte, dateCreated, dateModified time.Time) *FileInfo {
	return &FileInfo{
		FileName:     fileName,
		Offset:       offset,
		Size:         size,
		Hash:         hash,
		DateCreated:  dateCreated,
		DateModified: dateModified,
	}
}

// FileExtension returns the lowercase extension without the leading dot
func (f *FileInfo) FileExtension() string {
	if f == nil {
		return ""
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(strings.TrimSpace(f.FileName))), ".")
	return ext
}

// FileNameSanitized returns a path-safe filename
func (f *FileInfo) FileNameSanitized() string {
	if f == nil {
		return ""
	}
	return sanitizeFilename(f.FileName)
}

// IsAllowedExtension reports whether the file extension is allowed in plain .osz packages
func (f *FileInfo) IsAllowedExtension() bool {
	if f == nil {
		return false
	}
	_, ok := allowedFileExtensions[f.FileExtension()]
	return ok
}

// IsVideo reports whether the file is treated as a video file
func (f *FileInfo) IsVideo() bool {
	if f == nil {
		return false
	}
	_, ok := videoFileExtensions[f.FileExtension()]
	return ok
}

// IsBeatmap reports whether the file is an .osu beatmap file
func (f *FileInfo) IsBeatmap() bool {
	return f != nil && f.FileExtension() == "osu"
}

// IsCombinedBeatmap reports whether the file is an .osc combined beatmap file
func (f *FileInfo) IsCombinedBeatmap() bool {
	return f != nil && f.FileExtension() == "osc"
}
