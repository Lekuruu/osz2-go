// Package osz2 exposes osu! package files as standard Go filesystems.
package osz2

import (
	"cmp"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
)

// Metadata contains package metadata.
type Metadata map[MetaType]string

// PackageInfo describes the package header.
type PackageInfo struct {
	KeyType      KeyType
	Version      byte
	EncodedIV    [16]byte
	MetadataHash [md5.Size]byte
	FileInfoHash [md5.Size]byte
	BodyHash     [md5.Size]byte
}

// Reader is an osz2 / osf2 filesystem.
type Reader struct {
	source   io.ReaderAt
	size     int64
	info     PackageInfo
	metadata Metadata
	key      [md5.Size]byte
	dataBase int64

	entries  map[string]*Entry
	children map[string][]*Entry
	beatmaps map[int32]*Entry
}

// ReadCloser is a Reader that owns the io.Closer.
type ReadCloser struct {
	*Reader
	handle io.Closer
}

// Close closes the io.Closer handle, e.g. a file.
func (r *ReadCloser) Close() error {
	if r == nil || r.handle == nil {
		return nil
	}
	err := r.handle.Close()
	r.handle = nil
	return err
}

// OpenReader opens a file as an osz2 / osf2 filesystem.
func OpenReader(name string, keyType KeyType) (*ReadCloser, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	reader, err := NewReader(file, info.Size(), keyType)
	if err != nil {
		file.Close()
		return nil, err
	}
	return &ReadCloser{Reader: reader, handle: file}, nil
}

// NewReader returns an osz2 / osf2 filesystem from a reader source.
func NewReader(source io.ReaderAt, size int64, keyType KeyType) (*Reader, error) {
	if source == nil {
		return nil, fmt.Errorf("osz2: nil package source")
	}
	if size < 0 {
		return nil, fmt.Errorf("osz2: invalid package size %d", size)
	}

	reader := &Reader{
		source:   source,
		size:     size,
		info:     PackageInfo{KeyType: keyType},
		metadata: make(Metadata),
		entries:  make(map[string]*Entry),
		children: make(map[string][]*Entry),
		beatmaps: make(map[int32]*Entry),
	}
	// Read everything from metadata, beatmap IDs & entry table
	if err := reader.readIndex(); err != nil {
		return nil, err
	}
	// Build the directory tree from our newly read entries
	if err := reader.buildDirectories(); err != nil {
		return nil, err
	}
	return reader, nil
}

// Info returns a copy of the package header information.
func (r *Reader) Info() PackageInfo {
	return r.info
}

// Metadata returns a copy of the package metadata.
func (r *Reader) Metadata() Metadata {
	return maps.Clone(r.metadata)
}

// Open implements fs.FS.
func (r *Reader) Open(name string) (fs.File, error) {
	return r.OpenEntry(name)
}

// OpenEntry opens the File from the given name.
func (r *Reader) OpenEntry(name string) (*File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	entry, ok := r.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &File{reader: r, entry: entry}, nil
}

// OpenBeatmap opens the File assigned to the given beatmap ID.
func (r *Reader) OpenBeatmap(id int32) (*File, error) {
	entry, ok := r.beatmaps[id]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: fmt.Sprintf("beatmap:%d", id), Err: fs.ErrNotExist}
	}
	return r.OpenEntry(entry.path)
}

// Stat implements fs.StatFS.
func (r *Reader) Stat(name string) (fs.FileInfo, error) {
	entry, err := r.Entry(name)
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// Entry returns file metadata without opening its contents.
func (r *Reader) Entry(name string) (*Entry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "entry", Path: name, Err: fs.ErrInvalid}
	}
	entry, ok := r.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "entry", Path: name, Err: fs.ErrNotExist}
	}
	return entry, nil
}

// EntryForBeatmap returns the file metadata assigned to the given beatmap ID.
func (r *Reader) EntryForBeatmap(id int32) (*Entry, bool) {
	entry, ok := r.beatmaps[id]
	return entry, ok
}

// ReadDir implements fs.ReadDirFS.
func (r *Reader) ReadDir(name string) ([]fs.DirEntry, error) {
	entry, err := r.Entry(name)
	if err != nil {
		return nil, err
	}
	if !entry.isDir {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fmt.Errorf("%w: not a directory", fs.ErrInvalid)}
	}

	children := r.children[name]
	result := make([]fs.DirEntry, len(children))
	for i, child := range children {
		result[i] = child
	}
	return result, nil
}

// Verify checks the encrypted package body & every
// regular file without loading them all into memory.
func (r *Reader) Verify() error {
	if err := r.verifyBodyHash(); err != nil {
		return err
	}

	paths := make([]string, 0, len(r.entries))
	for name, entry := range r.entries {
		if !entry.isDir {
			paths = append(paths, name)
		}
	}
	slices.Sort(paths)

	for _, name := range paths {
		file, err := r.OpenEntry(name)
		if err != nil {
			return err
		}

		verifyErr := file.Verify()
		closeErr := file.Close()
		if verifyErr != nil || closeErr != nil {
			return errors.Join(verifyErr, closeErr)
		}
	}
	return nil
}

func (r *Reader) verifyBodyHash() error {
	total := r.size - r.dataBase
	excludeStart, excludeLength := bodyHashExclusion(r.metadata, total)

	hasher := newBodyHasher(
		total,
		excludeStart,
		excludeLength,
	)
	if _, err := io.Copy(hasher, io.NewSectionReader(r.source, r.dataBase, total)); err != nil {
		return fmt.Errorf("osz2: verify package body: %w", err)
	}
	if hasher.sum() != r.info.BodyHash {
		return errors.New("osz2: package body hash mismatch")
	}
	return nil
}

func (r *Reader) buildDirectories() error {
	/*
		Given our read entries, we effentively want
		something like this in the end:

		r.children["."] = {
			"song.mp3",
			"images",
			"maps",
		}
		r.children["images"] = {
			"bg.jpg",
		}
		r.children["maps"] = {
			"easy.osu",
			"hard.osu",
		}
	*/

	// Create the root directory
	r.entries["."] = newDirectoryEntry(".")

	// Take a snapshot so newly created parent
	// directories are not traversed again
	entries := slices.Collect(maps.Values(r.entries))

	for _, entry := range entries {
		if entry.path == "." || entry.isDir {
			continue
		}

		// We want to create every sub-directory of the package
		// To do this, we walk the path upwards until we have reached the "." directory
		parent := path.Dir(entry.path)
		for parent != "." {
			existing, exists := r.entries[parent]
			if !exists {
				r.entries[parent] = newDirectoryEntry(parent)
			} else if !existing.isDir {
				return fmt.Errorf("osz2: entry '%q' is both a file and a directory", parent) // pepega
			}
			parent = path.Dir(parent)
		}
	}

	// Now that we have all directories listed, we can start indexing parent -> children relationships
	for entryPath, entry := range r.entries {
		if entryPath == "." {
			continue
		}
		parent := path.Dir(entryPath)
		r.children[parent] = append(r.children[parent], entry)
	}
	for directory := range r.children {
		slices.SortFunc(r.children[directory], func(a, b *Entry) int {
			return cmp.Compare(a.Name(), b.Name())
		})
	}
	return nil
}
