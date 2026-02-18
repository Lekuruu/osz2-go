package osz2

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Package represents an osz2 package
type Package struct {
	IV      []byte
	Version byte

	// Metadata contains .osu metadata (e.g Artist, Difficulty, etc..)
	Metadata map[MetaType]string

	// FileInfos contains .osu file info (e.g FileName, Hash, Size etc..)
	FileInfos map[string]*FileInfo

	// FileNames maps filename to beatmap id
	FileNames map[string]int32

	// FileIDs maps beatmap id to filename
	FileIDs map[int32]string

	// Hashes
	MetaDataHash []byte
	FileInfoHash []byte
	FullBodyHash []byte

	// Key for XTEA algorithm
	key []byte

	// KeyType controls the key derivation
	KeyType KeyType

	// Want to read the metadata only?
	metadataOnly bool
}

// NewPackage creates a new osz2 package from a reader
func NewPackage(r io.ReadSeeker, metadataOnly bool) (*Package, error) {
	return NewPackageWithKeyType(r, metadataOnly, KeyTypeOSZ2)
}

// NewPackageWithKeyType creates a new package from a reader with a specific key derivation
func NewPackageWithKeyType(r io.ReadSeeker, metadataOnly bool, keyType KeyType) (*Package, error) {
	p := &Package{
		Metadata:     make(map[MetaType]string),
		FileInfos:    make(map[string]*FileInfo),
		FileNames:    make(map[string]int32),
		FileIDs:      make(map[int32]string),
		Version:      0,
		IV:           make([]byte, 16),
		KeyType:      keyType,
		metadataOnly: metadataOnly,
	}

	err := p.read(r)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// NewPackageFromFile reads a package directly from a file path
func NewPackageFromFile(path string, metadataOnly bool, keyType KeyType) (*Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return NewPackageWithKeyType(f, metadataOnly, keyType)
}

// NewPackageFromBytes reads a package from raw bytes.
func NewPackageFromBytes(data []byte, metadataOnly bool, keyType KeyType) (*Package, error) {
	return NewPackageWithKeyType(bytes.NewReader(data), metadataOnly, keyType)
}

// NewPackageFromDirectory initializes a package from all files in a directory
func NewPackageFromDirectory(directory string, keyType KeyType) (*Package, error) {
	p := NewEmptyPackage(keyType)
	if err := p.AddDirectory(directory, true); err != nil {
		return nil, err
	}
	return p, nil
}

// NewEmptyPackage creates an editable package without reading from a source file
func NewEmptyPackage(keyType KeyType) *Package {
	return &Package{
		Metadata:  make(map[MetaType]string),
		FileInfos: make(map[string]*FileInfo),
		FileNames: make(map[string]int32),
		FileIDs:   make(map[int32]string),
		Version:   0,
		IV:        make([]byte, 16),
		KeyType:   keyType,
	}
}

// Files returns package file contents as a filename to byte-array map
func (p *Package) Files() map[string][]byte {
	result := make(map[string][]byte, len(p.FileInfos))
	for name, info := range p.FileInfos {
		if info == nil || info.Content == nil {
			continue
		}
		result[name] = info.Content
	}
	return result
}

// FindFileByName gets file content by filename
func (p *Package) FindFileByName(name string) ([]byte, bool) {
	info, ok := p.FileInfos[name]
	if !ok {
		return nil, false
	}
	if info == nil || info.Content == nil {
		return nil, false
	}
	return info.Content, true
}

// FindFileByBeatmapID gets file content by beatmap ID
func (p *Package) FindFileByBeatmapID(beatmapID int32) (string, []byte, bool) {
	if name, ok := p.FileIDs[beatmapID]; ok {
		info := p.FileInfos[name]
		if info == nil {
			return name, nil, false
		}
		return name, info.Content, info.Content != nil
	}
	return "", nil, false
}

// AddFile adds or replaces a file in the package
func (p *Package) AddFile(filename string, content []byte, dateCreated, dateModified time.Time) *FileInfo {
	if dateCreated.IsZero() {
		dateCreated = time.Now().UTC()
	}
	if dateModified.IsZero() {
		dateModified = time.Now().UTC()
	}

	info := NewFileInfo(filename, 0, int32(len(content)+4), nil, dateCreated, dateModified)
	info.Content = content
	p.AddFileInfo(info)
	return info
}

// AddFileInfo adds or replaces a FileInfo in the package
func (p *Package) AddFileInfo(info *FileInfo) {
	p.FileInfos[info.FileName] = info

	if info.IsBeatmap() {
		if _, exists := p.FileNames[info.FileName]; !exists {
			p.FileNames[info.FileName] = -1
			p.FileIDs[-1] = info.FileName
			info.BeatmapID = -1
		}
		if beatmapID, exists := p.FileNames[info.FileName]; exists {
			info.BeatmapID = beatmapID
		}
	}
}

// AddFileFromDisk adds a file from disk
func (p *Package) AddFileFromDisk(filename, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	p.AddFile(filename, content, st.ModTime(), st.ModTime())
	return nil
}

// AddDirectory adds files from a directory
func (p *Package) AddDirectory(path string, recursive bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	base, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	if !recursive {
		entries, err := os.ReadDir(base)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fullPath := filepath.Join(base, entry.Name())
			if err := p.AddFileFromDisk(entry.Name(), fullPath); err != nil {
				return err
			}
		}
		return nil
	}

	return filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		return p.AddFileFromDisk(relPath, path)
	})
}

// RemoveFile removes a file from the package
func (p *Package) RemoveFile(filename string) bool {
	_, ok := p.FileInfos[filename]
	if !ok {
		return false
	}
	delete(p.FileInfos, filename)

	if beatmapID, exists := p.FileNames[filename]; exists {
		delete(p.FileNames, filename)
		delete(p.FileIDs, beatmapID)
	}
	return true
}

// AddMetadata adds or updates metadata
func (p *Package) AddMetadata(metaType MetaType, value any) {
	p.Metadata[metaType] = fmt.Sprint(value)
}

// RemoveMetadata removes metadata
func (p *Package) RemoveMetadata(metaType MetaType) bool {
	if _, ok := p.Metadata[metaType]; !ok {
		return false
	}
	delete(p.Metadata, metaType)
	return true
}

// GetMetadata gets metadata by type
func (p *Package) GetMetadata(metaType MetaType) (string, bool) {
	v, ok := p.Metadata[metaType]
	return v, ok
}

// SetBeatmapID sets beatmap ID for a .osu file
func (p *Package) SetBeatmapID(filename string, beatmapID int32) error {
	if _, ok := p.FileInfos[filename]; !ok {
		return fmt.Errorf("file not found: %s", filename)
	}

	info := p.FileInfos[filename]
	if info == nil {
		return fmt.Errorf("file info is nil: %s", filename)
	}
	if !info.IsBeatmap() {
		return fmt.Errorf("file is not a beatmap: %s", filename)
	}

	if oldID, ok := p.FileNames[filename]; ok {
		delete(p.FileIDs, oldID)
	}
	p.FileNames[filename] = beatmapID
	p.FileIDs[beatmapID] = filename
	if info, exists := p.FileInfos[filename]; exists && info != nil {
		info.BeatmapID = beatmapID
	}
	return nil
}

// SetBeatmapSetID sets BeatmapSetID metadata
func (p *Package) SetBeatmapSetID(beatmapSetID int64) {
	p.Metadata[BeatmapSetID] = strconv.FormatInt(beatmapSetID, 10)
}

// CreateOszPackage creates a plain .osz zip package from the current files
func (p *Package) CreateOszPackage(excludeDisallowedFiles bool) ([]byte, error) {
	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)
	defer zw.Close()

	for _, info := range p.FileInfos {
		if info == nil {
			continue
		}
		content := info.Content
		if content == nil {
			continue
		}
		if excludeDisallowedFiles {
			if !info.IsAllowedExtension() {
				continue
			}
		}

		hdr := &zip.FileHeader{
			Name:     filepath.ToSlash(info.FileNameSanitized()),
			Method:   zip.Deflate,
			Modified: time.Now(),
		}

		if !info.DateModified.IsZero() {
			hdr.Modified = info.DateModified
		}

		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(content); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
