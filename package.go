package osz2

import (
	"bytes"
	"io"
	"os"
)

// Package represents an osz2 package
type Package struct {
	IV      []byte
	Version byte

	// Metadata contains .osu metadata (e.g Artist, Difficulty, etc..)
	Metadata map[MetaType]string

	// FileInfos contains .osu file info (e.g FileName, Hash, Size etc..)
	FileInfos map[string]*FileInfo

	// Files contains osz2 file contents
	Files map[string][]byte

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

	// Need decrypt only metadata?
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
		Files:        make(map[string][]byte),
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

// NewPackageFromFile reads a package directly from a file path.
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

// NewEmptyPackage creates an editable package without reading from a source file.
func NewEmptyPackage(keyType KeyType) *Package {
	return &Package{
		Metadata:  make(map[MetaType]string),
		FileInfos: make(map[string]*FileInfo),
		Files:     make(map[string][]byte),
		FileNames: make(map[string]int32),
		FileIDs:   make(map[int32]string),
		Version:   0,
		IV:        make([]byte, 16),
		KeyType:   keyType,
	}
}
