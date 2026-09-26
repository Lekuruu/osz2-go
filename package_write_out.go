package osz2

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/Lekuruu/osz2-go/internal/crypto"
)

type fullWriter struct {
	io.Writer
}

func (w fullWriter) Write(data []byte) (int, error) {
	return writeFull(w.Writer, data)
}

func (w *Writer) writeHeader(metadataHash, fileInfoHash, bodyHash, encodedIV [md5.Size]byte) error {
	if _, err := w.destination.Write(packageMagic[:]); err != nil {
		return err
	}
	if err := binary.Write(w.destination, binary.LittleEndian, w.version); err != nil {
		return err
	}
	if _, err := w.destination.Write(encodedIV[:]); err != nil {
		return err
	}
	if _, err := w.destination.Write(metadataHash[:]); err != nil {
		return err
	}
	if _, err := w.destination.Write(fileInfoHash[:]); err != nil {
		return err
	}
	if _, err := w.destination.Write(bodyHash[:]); err != nil {
		return err
	}
	return nil
}

func (w *Writer) writeBeatmapMappings() error {
	var beatmaps []*writeEntry
	for _, entry := range w.entries {
		if entry.isBeatmapFile() {
			beatmaps = append(beatmaps, entry)
		}
	}

	if err := binary.Write(w.destination, binary.LittleEndian, int32(len(beatmaps))); err != nil {
		return err
	}

	for _, entry := range beatmaps {
		if err := writeString(w.destination, entry.path); err != nil {
			return err
		}
		if err := binary.Write(w.destination, binary.LittleEndian, entry.beatmapID); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) buildFileInfo() ([]byte, error) {
	writer := crypto.NewXXTEAWriter(bytesToUint32s(w.key[:]))

	if err := binary.Write(writer, binary.LittleEndian, int32(len(w.entries))); err != nil {
		return nil, err
	}
	if err := binary.Write(writer, binary.LittleEndian, w.entries[0].offset); err != nil {
		return nil, err
	}

	for i, entry := range w.entries {
		if err := writeString(writer, entry.path); err != nil {
			return nil, err
		}
		if _, err := writer.Write(entry.hash[:]); err != nil {
			return nil, err
		}

		createdAt, err := datetimeToDotNetBinary(entry.createdAt)
		if err != nil {
			return nil, err
		}
		modifiedAt, err := datetimeToDotNetBinary(entry.modifiedAt)
		if err != nil {
			return nil, err
		}

		if err := binary.Write(writer, binary.LittleEndian, createdAt); err != nil {
			return nil, err
		}
		if err := binary.Write(writer, binary.LittleEndian, modifiedAt); err != nil {
			return nil, err
		}

		if i+1 < len(w.entries) {
			if err := binary.Write(
				writer,
				binary.LittleEndian,
				w.entries[i+1].offset,
			); err != nil {
				return nil, err
			}
		}
	}

	return bytes.Clone(writer.Bytes()), nil
}

func encodeMetadata(metadata Metadata) ([]byte, error) {
	keys := slices.Sorted(maps.Keys(metadata))
	keysAmount := len(keys)

	var buffer bytes.Buffer

	if err := binary.Write(&buffer, binary.LittleEndian, int32(keysAmount)); err != nil {
		return nil, err
	}

	for _, key := range keys {
		if err := binary.Write(&buffer, binary.LittleEndian, int16(key)); err != nil {
			return nil, err
		}
		if err := writeString(&buffer, metadata[key]); err != nil {
			return nil, err
		}
	}

	return buffer.Bytes(), nil
}

func (w *Writer) calculateBodyHash() ([md5.Size]byte, error) {
	var size int64
	for _, entry := range w.entries {
		size += 4 + entry.size
	}

	excludeStart, excludeLength := bodyHashExclusion(w.metadata, size)
	hasher := newBodyHasher(size, excludeStart, excludeLength)

	if err := w.streamFileData(hasher); err != nil {
		return [md5.Size]byte{}, err
	}

	return hasher.sum(), nil
}

func (w *Writer) streamFileData(destination io.Writer) error {
	key := bytesToUint32s(w.key[:])
	lengthCryptor := crypto.NewXXTEA(key)

	for _, entry := range w.entries {
		var length [4]byte
		binary.LittleEndian.PutUint32(length[:], uint32(entry.size))
		lengthCryptor.Encrypt(length[:], 0, len(length))

		if _, err := destination.Write(length[:]); err != nil {
			return err
		}

		file, err := w.source.Open(entry.path)
		if err != nil {
			return err
		}

		streamErr := streamEncryptedFrame(
			destination,
			file,
			entry.size,
			w.key,
			entry.hash,
		)
		closeErr := file.Close()

		if streamErr != nil {
			return fmt.Errorf("osz2: stream %q: %w", entry.path, streamErr)
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

func streamEncryptedFrame(
	destination io.Writer,
	source io.Reader,
	size int64,
	key [md5.Size]byte,
	expectedHash [md5.Size]byte,
) error {
	hasher := md5.New()
	cryptor := crypto.NewXXTEA(bytesToUint32s(key[:]))

	var buffer [crypto.MaxBytes]byte

	for remaining := size; remaining > 0; {
		blockSize := min(remaining, int64(len(buffer)))
		block := buffer[:blockSize]

		if _, err := io.ReadFull(source, block); err != nil {
			return err
		}

		hasher.Write(block)
		cryptor.Encrypt(block, 0, len(block))

		if _, err := destination.Write(block); err != nil {
			return err
		}
		remaining -= blockSize
	}

	// Make sure the file hasn't changed since AddFS indexed it
	var extra [1]byte
	n, err := source.Read(extra[:])
	if n != 0 || err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("source changed after indexing")
	}

	// We also hash the file again to catch changes that didn't affect its size
	if !bytes.Equal(hasher.Sum(nil), expectedHash[:]) {
		return errors.New("source content changed after indexing")
	}
	return nil
}
