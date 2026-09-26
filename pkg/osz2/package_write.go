package osz2

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/Lekuruu/osz2-go/internal/crypto"
)

// Writer creates an osz2 / osf2 package from an fs.FS.
// Nothing is written until Close is called.
type Writer struct {
	keyType  KeyType
	version  byte
	metadata Metadata

	destination fullWriter
	source      fs.FS
	entries     []*writeEntry
	key         [md5.Size]byte
	indexed     bool
	closed      bool
}

type writeEntry struct {
	path   string
	size   int64
	offset int32
	hash   [md5.Size]byte

	createdAt  time.Time
	modifiedAt time.Time
	beatmapID  int32
	videoHash  [md5.Size]byte
}

// NewWriter creates a writer. duhhh!!
func NewWriter(destination io.Writer) (*Writer, error) {
	if destination == nil {
		return nil, errors.New("osz2: nil writer destination")
	}

	return &Writer{
		destination: fullWriter{Writer: destination},
		metadata:    make(Metadata),
	}, nil
}

// SetVersion sets the package format version.
// Must be called before SetFS.
func (w *Writer) SetVersion(version byte) error {
	if err := w.checkConfigurable(); err != nil {
		return err
	}
	w.version = version
	return nil
}

// SetKey selects the package key derivation scheme.
// Must be called before SetFS.
func (w *Writer) SetKey(keyType KeyType) error {
	if err := w.checkConfigurable(); err != nil {
		return err
	}
	if keyType != KeyTypeOsz2 && keyType != KeyTypeOsf2 {
		return errors.New("osz2: unsupported key type")
	}
	w.keyType = keyType
	return nil
}

// SetMetadata sets a package metadata value.
// Must be called before SetFS.
func (w *Writer) SetMetadata(metadataType MetaType, value string) error {
	if err := w.checkConfigurable(); err != nil {
		return err
	}
	w.metadata[metadataType] = value
	return nil
}

// AssignBeatmapID assigns a beatmap ID to a package entry.
// Use -1 for a beatmap that has not been assigned an ID yet.
func (w *Writer) AssignBeatmapID(name string, id int32) error {
	if w.closed {
		return fs.ErrClosed
	}
	if !fs.ValidPath(name) || name == "." {
		return &fs.PathError{Op: "assign beatmap ID", Path: name, Err: fs.ErrInvalid}
	}
	if !w.indexed {
		return errors.New("osz2: call SetFS before assigning beatmap IDs")
	}

	entry, ok := w.indexedEntry(name)
	if !ok {
		return &fs.PathError{Op: "assign beatmap ID", Path: name, Err: fs.ErrNotExist}
	}
	if !entry.isBeatmapFile() {
		return fmt.Errorf("osz2: non-beatmap entry %q cannot have a beatmap ID", name)
	}

	previousID := entry.beatmapID
	entry.beatmapID = id
	if err := w.validateBeatmapIDs(); err != nil {
		entry.beatmapID = previousID
		return err
	}
	return nil
}

// AssignTimes assigns optional creation and modification times to a package entry.
// A zero modification time keeps the source file's resolved modification time.
// A zero creation time uses the resolved modification time.
func (w *Writer) AssignTimes(name string, createdAt, modifiedAt time.Time) error {
	if w.closed {
		return fs.ErrClosed
	}
	if !fs.ValidPath(name) || name == "." {
		return &fs.PathError{Op: "assign times", Path: name, Err: fs.ErrInvalid}
	}
	if !w.indexed {
		return errors.New("osz2: call SetFS before assigning entry times")
	}

	entry, ok := w.indexedEntry(name)
	if !ok {
		return &fs.PathError{Op: "assign times", Path: name, Err: fs.ErrNotExist}
	}
	if modifiedAt.IsZero() {
		modifiedAt = entry.modifiedAt
	}
	if createdAt.IsZero() {
		createdAt = modifiedAt
	}
	createdAt = createdAt.UTC()
	modifiedAt = modifiedAt.UTC()

	if err := validateEntryTimes(name, createdAt, modifiedAt); err != nil {
		return err
	}

	entry.createdAt = createdAt
	entry.modifiedAt = modifiedAt
	return nil
}

func (w *Writer) checkConfigurable() error {
	if w.closed {
		return fs.ErrClosed
	}
	if w.indexed {
		return errors.New("osz2: writer settings cannot change after SetFS")
	}
	return nil
}

func (w *Writer) indexedEntry(name string) (*writeEntry, bool) {
	for _, entry := range w.entries {
		if entry.path == name {
			return entry, true
		}
	}
	return nil, false
}

// SetFS scans the source fs and collects the information needed to write it.
// The filesystem must be usable until Close() is called.
func (w *Writer) SetFS(source fs.FS) error {
	if w.closed {
		return fs.ErrClosed
	}
	if w.indexed {
		return errors.New("osz2: writer already has a source filesystem")
	}
	if source == nil {
		return errors.New("osz2: nil source filesystem")
	}
	if w.keyType == "" {
		return errors.New("osz2: writer key is not set")
	}

	now := time.Now().UTC()
	var entries []*writeEntry

	err := fs.WalkDir(source, ".", func(name string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry.IsDir() {
			return nil
		}

		entry, err := w.indexSourceEntry(source, name, dirEntry, now)
		if err != nil {
			return err
		}

		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return errors.New("osz2: cannot write an empty filesystem")
	}
	w.entries = entries

	if err := w.finalizeIndex(); err != nil {
		return err
	}

	key, err := w.keyType.Generate(w.metadata)
	if err != nil {
		return fmt.Errorf("osz2: derive encryption key: %w", err)
	}

	w.key = key
	w.source = source
	w.indexed = true
	return nil
}

// Close writes the finished package.
// It does not close the destination actually.
func (w *Writer) Close() error {
	if w.closed {
		return fs.ErrClosed
	}
	w.closed = true

	if !w.indexed {
		return errors.New("osz2: writer has no source filesystem")
	}
	if err := w.validateBeatmapIDs(); err != nil {
		return err
	}

	fileInfo, err := w.buildFileInfo()
	if err != nil {
		return err
	}
	fileInfoHash := computeFileInfoHash(fileInfo, len(w.entries))

	metadata, err := encodeMetadata(w.metadata)
	if err != nil {
		return err
	}
	metadataHash := computeMetadataHash(metadata, len(w.metadata))

	bodyHash, err := w.calculateBodyHash()
	if err != nil {
		return err
	}

	var iv [md5.Size]byte
	if _, err := rand.Read(iv[:]); err != nil {
		return err
	}

	encodedIV := iv
	for i := range encodedIV {
		encodedIV[i] ^= bodyHash[i]
	}

	if err := w.writeHeader(metadataHash, fileInfoHash, bodyHash, encodedIV); err != nil {
		return err
	}
	if _, err := w.destination.Write(metadata); err != nil {
		return err
	}
	if err := w.writeBeatmapMappings(); err != nil {
		return err
	}

	magic := bytes.Clone(knownPlain)
	crypto.NewXTEA(bytesToUint32s(w.key[:])).Encrypt(magic, 0, len(magic))

	if _, err := w.destination.Write(magic); err != nil {
		return err
	}

	encodedLength, err := encodeFileInfoLength(len(fileInfo), fileInfoHash)
	if err != nil {
		return err
	}

	if err := binary.Write(w.destination, binary.LittleEndian, encodedLength); err != nil {
		return err
	}
	if _, err := w.destination.Write(fileInfo); err != nil {
		return err
	}

	return w.streamFileData(w.destination)
}
