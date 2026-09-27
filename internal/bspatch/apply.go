// SPDX-License-Identifier: BSD-2-Clause AND MIT
// SPDX-FileCopyrightText: 2003-2005 Colin Percival
// SPDX-FileCopyrightText: 2019 Gabriel Ochsenhofer
// SPDX-FileCopyrightText: 2025 TotallyGamerJet

package bspatch

import (
	"errors"
	"fmt"
	"io"
	"math"
)

const streamBufferSize = 32 * 1024

func applyPatch(
	oldBinary io.ReaderAt,
	oldSize int64,
	output io.Writer,
	controlReader, diffReader, extraReader io.Reader,
	newSize int64,
) error {
	if newSize == 0 {
		return nil
	}

	diffBuffer := make([]byte, streamBufferSize)
	oldBuffer := make([]byte, streamBufferSize)

	var oldPosition int64
	var newPosition int64
	var encodedInteger [8]byte
	var control [3]int64

	for newPosition < newSize {
		for i := range control {
			if _, err := io.ReadFull(controlReader, encodedInteger[:]); err != nil {
				return fmt.Errorf("corrupt patch: read control data: %w", err)
			}
			control[i] = decodeInt64(encodedInteger[:])
		}
		if control[0] < 0 {
			return errors.New("corrupt patch: negative diff length")
		}
		if control[1] < 0 {
			return errors.New("corrupt patch: negative extra length")
		}
		if control[0] == 0 && control[1] == 0 {
			return errors.New("corrupt patch: control data does not advance output")
		}
		if control[0] > newSize-newPosition {
			return errors.New("corrupt patch: diff data exceeds output size")
		}

		oldAfterDiff, ok := addInt64(oldPosition, control[0])
		if !ok {
			return errors.New("corrupt patch: old position overflow")
		}
		if err := writeDiffData(
			output,
			oldBinary,
			oldSize,
			diffReader,
			oldPosition,
			control[0],
			diffBuffer,
			oldBuffer,
		); err != nil {
			return err
		}
		newPosition += control[0]
		oldPosition = oldAfterDiff

		if control[1] > newSize-newPosition {
			return errors.New("corrupt patch: extra data exceeds output size")
		}
		if err := writeExtraData(output, extraReader, control[1], diffBuffer); err != nil {
			return err
		}
		newPosition += control[1]

		oldPosition, ok = addInt64(oldPosition, control[2])
		if !ok {
			return errors.New("corrupt patch: old position overflow")
		}
	}

	return nil
}

func writeDiffData(
	output io.Writer,
	oldBinary io.ReaderAt,
	oldSize int64,
	diffReader io.Reader,
	oldPosition, length int64,
	diffBuffer, oldBuffer []byte,
) error {
	var processed int64
	for processed < length {
		chunkSize := int(min(int64(len(diffBuffer)), length-processed))
		diffChunk := diffBuffer[:chunkSize]
		if _, err := io.ReadFull(diffReader, diffChunk); err != nil {
			return fmt.Errorf("corrupt patch: read diff data: %w", err)
		}

		chunkOldStart, ok := addInt64(oldPosition, processed)
		if !ok {
			return errors.New("corrupt patch: old position overflow")
		}
		chunkOldEnd, ok := addInt64(chunkOldStart, int64(chunkSize))
		if !ok {
			return errors.New("corrupt patch: old position overflow")
		}

		overlapStart := max(chunkOldStart, int64(0))
		overlapEnd := min(chunkOldEnd, oldSize)

		if overlapStart < overlapEnd {
			destinationOffset := int(overlapStart - chunkOldStart)
			oldLength := int(overlapEnd - overlapStart)
			oldChunk := oldBuffer[:oldLength]

			if _, err := io.ReadFull(
				io.NewSectionReader(oldBinary, overlapStart, int64(oldLength)),
				oldChunk,
			); err != nil {
				return fmt.Errorf("read old binary: %w", err)
			}
			for i := range oldLength {
				diffChunk[destinationOffset+i] += oldChunk[i]
			}
		}

		if err := writeAll(output, diffChunk); err != nil {
			return fmt.Errorf("write patched data: %w", err)
		}
		processed += int64(chunkSize)
	}
	return nil
}

func writeExtraData(output io.Writer, extraReader io.Reader, length int64, buffer []byte) error {
	var written int64
	for written < length {
		chunkSize := int(min(int64(len(buffer)), length-written))
		chunk := buffer[:chunkSize]

		if _, err := io.ReadFull(extraReader, chunk); err != nil {
			return fmt.Errorf("corrupt patch: read extra data: %w", err)
		}
		if err := writeAll(output, chunk); err != nil {
			return fmt.Errorf("write patched data: %w", err)
		}
		written += int64(chunkSize)
	}
	return nil
}

func writeAll(output io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := output.Write(data)
		if written < 0 || written > len(data) {
			return fmt.Errorf("invalid write count %d", written)
		}

		data = data[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func addInt64(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, false
	}
	if right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}
