package osz2

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Lekuruu/osz2-go/internal/bspatch"
)

// OpenPatch applies an osu! BSDIFF40 patch and
// opens the resulting osz2 / osf2 filesystem.
func OpenPatch(
	source io.ReaderAt,
	sourceSize int64,
	patch io.ReaderAt,
	patchSize int64,
	maxOutputSize int64,
	keyType KeyType,
) (*ReadCloser, error) {
	if source == nil {
		return nil, errors.New("osz2: nil package source")
	}
	if sourceSize < 0 {
		return nil, fmt.Errorf("osz2: invalid package size %d", sourceSize)
	}
	if patch == nil {
		return nil, errors.New("osz2: nil patch source")
	}
	if patchSize < 0 {
		return nil, fmt.Errorf("osz2: invalid patch size %d", patchSize)
	}
	if maxOutputSize < 0 {
		return nil, fmt.Errorf("osz2: invalid maximum output size %d", maxOutputSize)
	}

	// Stream the patched data to a temporary file
	file, err := os.CreateTemp("", "osz2-patch-*")
	if err != nil {
		return nil, fmt.Errorf("osz2: create temporary patch file: %w", err)
	}
	temporary := &temporaryFile{File: file}
	cleanup := func(original error) (*ReadCloser, error) {
		return nil, errors.Join(original, temporary.Close())
	}

	size, err := bspatch.Patch(
		sizedReaderAt{ReaderAt: source, size: sourceSize},
		sizedReaderAt{ReaderAt: patch, size: patchSize},
		temporary,
		maxOutputSize,
		bspatch.GzipReader,
	)
	if err != nil {
		return cleanup(fmt.Errorf("osz2: apply patch: %w", err))
	}

	// Open the patched package as an osz2 / osf2 filesystem
	reader, err := NewReader(temporary, size, keyType)
	if err != nil {
		return cleanup(err)
	}
	return &ReadCloser{Reader: reader, handle: temporary}, nil
}

type sizedReaderAt struct {
	io.ReaderAt
	size int64
}

func (r sizedReaderAt) Size() int64 {
	return r.size
}

type temporaryFile struct {
	*os.File
}

func (f *temporaryFile) Close() error {
	name := f.Name()
	return errors.Join(f.File.Close(), os.Remove(name))
}
