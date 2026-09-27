// SPDX-License-Identifier: BSD-2-Clause AND MIT
// SPDX-FileCopyrightText: 2003-2005 Colin Percival
// SPDX-FileCopyrightText: 2019 Gabriel Ochsenhofer
// SPDX-FileCopyrightText: 2025 TotallyGamerJet

// Package bspatch applies BSDIFF40 binary patches.
package bspatch

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	headerSize      = 32
	magic           = "BSDIFF40"
	integerSignMask = uint64(1) << 63
)

// SizedReaderAt provides ReadAt access to data with a known size.
type SizedReaderAt interface {
	io.ReaderAt
	Size() int64
}

// Decompressor creates a reader for one compressed patch section.
type Decompressor func(io.Reader) (io.Reader, error)

// Bzip2Reader creates a reader for a standard BSDIFF40 patch section.
func Bzip2Reader(source io.Reader) (io.Reader, error) {
	return bzip2.NewReader(source), nil
}

// GzipReader creates a reader for an osu! BSDIFF40 patch section.
func GzipReader(source io.Reader) (io.Reader, error) {
	return gzip.NewReader(source)
}

// PatchBytes applies an in-memory BSDIFF40 patch.
func PatchBytes(oldBinary, patch []byte, maxOutputSize int64) ([]byte, error) {
	var output bytes.Buffer
	_, err := Patch(
		bytes.NewReader(oldBinary),
		bytes.NewReader(patch),
		&output,
		maxOutputSize,
		Bzip2Reader,
	)
	if err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Patch applies a patch while streaming the result to output and returns the
// patched output size.
func Patch(
	oldBinary, patch SizedReaderAt,
	output io.Writer,
	maxOutputSize int64,
	decompress Decompressor,
) (outputSize int64, err error) {
	if maxOutputSize < 0 {
		return 0, errors.New("maximum output size cannot be negative")
	}
	if oldBinary == nil {
		return 0, errors.New("old binary reader cannot be nil")
	}
	if patch == nil {
		return 0, errors.New("patch reader cannot be nil")
	}
	if output == nil {
		return 0, errors.New("output writer cannot be nil")
	}
	if decompress == nil {
		return 0, errors.New("decompressor cannot be nil")
	}

	oldSize := oldBinary.Size()
	if oldSize < 0 {
		return 0, errors.New("old binary size cannot be negative")
	}
	patchSize := patch.Size()
	if patchSize < headerSize {
		return 0, errors.New("corrupt patch: shorter than header")
	}

	//	File format:
	//		0	8	"BSDIFF40"
	//		8	8	X
	//		16	8	Y
	//		24	8	sizeof(newfile)
	//		32	X	compressed control block
	//		32+X	Y	compressed diff block
	//		32+X+Y	???	compressed extra block

	var header [headerSize]byte
	if _, err := io.ReadFull(io.NewSectionReader(patch, 0, headerSize), header[:]); err != nil {
		return 0, fmt.Errorf("corrupt patch: read header: %w", err)
	}
	if string(header[:len(magic)]) != magic {
		return 0, errors.New("corrupt patch: invalid magic")
	}

	controlLength := decodeInt64(header[8:])
	diffLength := decodeInt64(header[16:])
	newSize := decodeInt64(header[24:])

	if controlLength < 0 || diffLength < 0 || newSize < 0 {
		return 0, fmt.Errorf(
			"corrupt patch (control length %v diff length %v new size %v)",
			controlLength,
			diffLength,
			newSize,
		)
	}
	if newSize > maxOutputSize {
		return 0, fmt.Errorf("patch output size %d exceeds limit %d", newSize, maxOutputSize)
	}

	controlOffset := int64(headerSize)
	if controlLength > patchSize-controlOffset {
		return 0, errors.New("corrupt patch: control block exceeds patch size")
	}
	diffOffset := controlOffset + controlLength
	if diffLength > patchSize-diffOffset {
		return 0, errors.New("corrupt patch: diff block exceeds patch size")
	}
	extraOffset := diffOffset + diffLength

	controlReader, err := decompress(io.NewSectionReader(patch, controlOffset, controlLength))
	if err != nil {
		return 0, fmt.Errorf("open control stream: %w", err)
	}
	if controlReader == nil {
		return 0, errors.New("open control stream: decompressor returned a nil reader")
	}
	defer func() { err = errors.Join(err, closeReader(controlReader)) }()

	diffReader, err := decompress(io.NewSectionReader(patch, diffOffset, diffLength))
	if err != nil {
		return 0, fmt.Errorf("open diff stream: %w", err)
	}
	if diffReader == nil {
		return 0, errors.New("open diff stream: decompressor returned a nil reader")
	}
	defer func() { err = errors.Join(err, closeReader(diffReader)) }()

	extraReader, err := decompress(io.NewSectionReader(patch, extraOffset, patchSize-extraOffset))
	if err != nil {
		return 0, fmt.Errorf("open extra stream: %w", err)
	}
	if extraReader == nil {
		return 0, errors.New("open extra stream: decompressor returned a nil reader")
	}
	defer func() { err = errors.Join(err, closeReader(extraReader)) }()

	if err := applyPatch(
		oldBinary, oldSize, output,
		controlReader, diffReader, extraReader, newSize,
	); err != nil {
		return 0, err
	}
	return newSize, nil
}

func closeReader(reader io.Reader) error {
	closer, ok := reader.(io.Closer)
	if !ok {
		return nil
	}
	return closer.Close()
}

// decodeInt64 decodes BSDIFF's 63-bit signed-magnitude integer format.
func decodeInt64(buf []byte) int64 {
	encoded := binary.LittleEndian.Uint64(buf)
	magnitude := int64(encoded &^ integerSignMask)
	if encoded&integerSignMask != 0 {
		return -magnitude
	}
	return magnitude
}
