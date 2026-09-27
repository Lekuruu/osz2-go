// SPDX-License-Identifier: BSD-2-Clause AND MIT
// SPDX-FileCopyrightText: 2003-2005 Colin Percival
// SPDX-FileCopyrightText: 2019 Gabriel Ochsenhofer
// SPDX-FileCopyrightText: 2025 TotallyGamerJet

// Package bspatch applies BSDIFF40 binary patches.
package bspatch

import (
	"bytes"
	"compress/bzip2"
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

// Patch applies patch to oldBinary and returns the resulting binary.
func Patch(oldBinary, patch []byte, maxOutputSize int64) ([]byte, error) {
	if maxOutputSize < 0 {
		return nil, errors.New("maximum output size cannot be negative")
	}

	//	File format:
	//		0	8	"BSDIFF40"
	//		8	8	X
	//		16	8	Y
	//		24	8	sizeof(newfile)
	//		32	X	bzip2(control block)
	//		32+X	Y	bzip2(diff block)
	//		32+X+Y	???	bzip2(extra block)
	//	with control block a set of triples (x,y,z) meaning "add x bytes
	//	from oldBinary to x bytes from the diff block; copy y bytes from the
	//	extra block; move by z bytes in oldBinary".

	patchReader := bytes.NewReader(patch)
	var header [headerSize]byte
	if _, err := io.ReadFull(patchReader, header[:]); err != nil {
		return nil, fmt.Errorf("corrupt patch: read header: %w", err)
	}
	if string(header[:len(magic)]) != magic {
		return nil, errors.New("corrupt patch: invalid magic")
	}

	controlLength := decodeInt64(header[8:])
	diffLength := decodeInt64(header[16:])
	newSize := decodeInt64(header[24:])

	if controlLength < 0 || diffLength < 0 || newSize < 0 {
		return nil, fmt.Errorf(
			"corrupt patch (control length %v diff length %v new size %v)",
			controlLength,
			diffLength,
			newSize,
		)
	}
	if newSize > maxOutputSize {
		return nil, fmt.Errorf("patch output size %d exceeds limit %d", newSize, maxOutputSize)
	}

	outputSize := int(newSize)
	if int64(outputSize) != newSize {
		return nil, fmt.Errorf("patch output size %d exceeds platform capacity", newSize)
	}

	patchSize := int64(len(patch))
	controlOffset := int64(headerSize)
	if controlLength > patchSize-controlOffset {
		return nil, errors.New("corrupt patch: control block exceeds patch size")
	}

	diffOffset := controlOffset + controlLength
	if diffLength > patchSize-diffOffset {
		return nil, errors.New("corrupt patch: diff block exceeds patch size")
	}

	extraOffset := diffOffset + diffLength
	controlReader := bzip2.NewReader(io.NewSectionReader(patchReader, controlOffset, controlLength))
	diffReader := bzip2.NewReader(io.NewSectionReader(patchReader, diffOffset, diffLength))
	extraReader := bzip2.NewReader(io.NewSectionReader(patchReader, extraOffset, patchSize-extraOffset))

	result := make([]byte, outputSize)
	oldSize := int64(len(oldBinary))

	var oldPosition int64
	var newPosition int64
	var encodedInteger [8]byte
	var control [3]int64

	for newPosition < newSize {
		for i := range control {
			if _, err := io.ReadFull(controlReader, encodedInteger[:]); err != nil {
				return nil, fmt.Errorf("corrupt patch: read control data: %w", err)
			}
			control[i] = decodeInt64(encodedInteger[:])
		}
		if control[0] < 0 {
			return nil, errors.New("corrupt patch: negative diff length")
		}
		if control[1] < 0 {
			return nil, errors.New("corrupt patch: negative extra length")
		}

		if newPosition+control[0] > newSize {
			return nil, errors.New("corrupt patch: diff data exceeds output size")
		}

		if _, err := io.ReadFull(diffReader, result[newPosition:newPosition+control[0]]); err != nil {
			return nil, fmt.Errorf("corrupt patch: read diff data: %w", err)
		}
		for i := range control[0] {
			if oldPosition+i >= 0 && oldPosition+i < oldSize {
				result[newPosition+i] += oldBinary[oldPosition+i]
			}
		}

		newPosition += control[0]
		oldPosition += control[0]

		if newPosition+control[1] > newSize {
			return nil, errors.New("corrupt patch: extra data exceeds output size")
		}

		if _, err := io.ReadFull(extraReader, result[newPosition:newPosition+control[1]]); err != nil {
			return nil, fmt.Errorf("corrupt patch: read extra data: %w", err)
		}
		newPosition += control[1]
		oldPosition += control[2]
	}

	return result, nil
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
