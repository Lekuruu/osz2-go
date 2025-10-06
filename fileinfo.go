package osz2

import "time"

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
