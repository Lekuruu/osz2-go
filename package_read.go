package osz2

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/Lekuruu/osz2-go/internal/crypto"
)

var packageMagic = [3]byte{0xec, 0x48, 0x4f}

func (r *Reader) readIndex() error {
	stream := io.NewSectionReader(r.source, 0, r.size)
	var magic [3]byte
	if _, err := io.ReadFull(stream, magic[:]); err != nil {
		return fmt.Errorf("osz2: read package magic: %w", err)
	}
	if magic != packageMagic {
		return errors.New("osz2: invalid package magic")
	}

	if err := binary.Read(stream, binary.LittleEndian, &r.info.Version); err != nil {
		return fmt.Errorf("osz2: read version: %w", err)
	}
	if _, err := io.ReadFull(stream, r.info.EncodedIV[:]); err != nil {
		return fmt.Errorf("osz2: read IV: %w", err)
	}

	if _, err := io.ReadFull(stream, r.info.MetadataHash[:]); err != nil {
		return fmt.Errorf("osz2: read metadata hash: %w", err)
	}
	if _, err := io.ReadFull(stream, r.info.FileInfoHash[:]); err != nil {
		return fmt.Errorf("osz2: read file-info hash: %w", err)
	}
	if _, err := io.ReadFull(stream, r.info.BodyHash[:]); err != nil {
		return fmt.Errorf("osz2: read body hash: %w", err)
	}

	if err := r.readMetadata(stream); err != nil {
		return err
	}

	beatmapIDs, err := readBeatmapIDs(stream)
	if err != nil {
		return err
	}

	r.key, err = r.info.KeyType.Generate(r.metadata)
	if err != nil {
		return fmt.Errorf("osz2: derive encryption key: %w", err)
	}
	if err := r.readEntries(stream, beatmapIDs); err != nil {
		return err
	}
	return nil
}

func (r *Reader) readMetadata(stream io.ReadSeeker) error {
	var raw bytes.Buffer
	var count int32

	if err := binary.Read(stream, binary.LittleEndian, &count); err != nil {
		return fmt.Errorf("osz2: read metadata count: %w", err)
	}
	if count < 0 {
		return fmt.Errorf("osz2: invalid metadata count %d", count)
	}

	remaining, err := remainingBytes(stream)
	if err != nil {
		return err
	}
	if int64(count) > remaining/3 { // sanity check to verify that the reported count is correct
		return fmt.Errorf("osz2: metadata count %d exceeds the remaining package data", count)
	}
	if err := binary.Write(&raw, binary.LittleEndian, count); err != nil {
		return err
	}

	for range count {
		var metadataType int16
		if err := binary.Read(stream, binary.LittleEndian, &metadataType); err != nil {
			return fmt.Errorf("osz2: read metadata type: %w", err)
		}

		value, err := readString(stream)
		if err != nil {
			return fmt.Errorf("osz2: read metadata value: %w", err)
		}
		if _, exists := r.metadata[MetaType(metadataType)]; exists {
			// TODO: Check how the c# version handles this, but I think this should be rejected
			return fmt.Errorf("osz2: duplicate metadata type %d", metadataType)
		}

		r.metadata[MetaType(metadataType)] = value
		if err := binary.Write(&raw, binary.LittleEndian, metadataType); err != nil {
			return err
		}
		if err := writeString(&raw, value); err != nil {
			return err
		}
	}

	// With the bytes we just collected we
	// can now verify the metadata checksum
	hash := computeOszHash(raw.Bytes(), int(count)*3, 0xa7)

	if hash != r.info.MetadataHash {
		return errors.New("osz2: metadata hash mismatch")
	}
	return nil
}

func readBeatmapIDs(stream io.ReadSeeker) (map[string]int32, error) {
	var count int32
	if err := binary.Read(stream, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("osz2: read beatmap count: %w", err)
	}
	if count < 0 {
		return nil, fmt.Errorf("osz2: invalid beatmap count %d", count)
	}

	remaining, err := remainingBytes(stream)
	if err != nil {
		return nil, err
	}
	if int64(count) > remaining/5 { // sanity check to verify that the count is correct
		return nil, fmt.Errorf("osz2: beatmap count %d exceeds the remaining package data", count)
	}

	result := make(map[string]int32, count)
	reverse := make(map[int32]string, count)

	for range count {
		name, err := readString(stream)
		if err != nil {
			return nil, fmt.Errorf("osz2: read beatmap filename: %w", err)
		}

		name, err = canonicalEntryPath(name)
		if err != nil {
			return nil, err
		}

		var id int32
		if err := binary.Read(stream, binary.LittleEndian, &id); err != nil {
			return nil, fmt.Errorf("osz2: read beatmap ID: %w", err)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("osz2: duplicate beatmap filename %q", name)
		}

		if id != -1 {
			if owner, exists := reverse[id]; exists {
				return nil, fmt.Errorf("osz2: beatmap ID %d is assigned to both %q and %q", id, owner, name)
			}
			reverse[id] = name
		}
		result[name] = id
	}
	return result, nil
}

func (r *Reader) readEntries(stream io.ReadSeeker, beatmapIDs map[string]int32) error {
	key := bytesToUint32s(r.key[:])
	encryptedMagic := make([]byte, len(knownPlain))
	if _, err := io.ReadFull(stream, encryptedMagic); err != nil {
		return fmt.Errorf("osz2: read encrypted magic: %w", err)
	}

	crypto.NewXTEA(key).Decrypt(encryptedMagic, 0, len(encryptedMagic))
	if !bytes.Equal(encryptedMagic, knownPlain) {
		return errors.New("osz2: invalid encryption key")
	}

	var encodedLength int32
	if err := binary.Read(stream, binary.LittleEndian, &encodedLength); err != nil {
		return fmt.Errorf("osz2: read file-info length: %w", err)
	}

	length := int64(encodedLength)
	for i := 0; i < md5.Size; i += 2 {
		length -= int64(r.info.FileInfoHash[i]) | int64(r.info.FileInfoHash[i+1])<<17
	}
	if length < 0 {
		return fmt.Errorf("osz2: invalid file-info length %d", length)
	}

	fileInfoStart, err := stream.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if int64(length) > r.size-fileInfoStart {
		return fmt.Errorf("osz2: file-info length %d exceeds remaining package data", length)
	}

	encryptedInfo := make([]byte, int(length))
	if _, err := io.ReadFull(stream, encryptedInfo); err != nil {
		return fmt.Errorf("osz2: read file info: %w", err)
	}
	r.dataBase = fileInfoStart + length
	if r.size-r.dataBase >= 1<<31 {
		return fmt.Errorf("osz2: encrypted file data is too large")
	}
	return r.parseFileInfo(encryptedInfo, beatmapIDs)
}

func (r *Reader) parseFileInfo(encryptedInfo []byte, beatmapIDs map[string]int32) error {
	decoder := crypto.NewXXTEAReader(
		bytes.NewReader(encryptedInfo),
		bytesToUint32s(r.key[:]),
	)

	var count int32
	if err := binary.Read(decoder, binary.LittleEndian, &count); err != nil {
		return fmt.Errorf("osz2: read file count: %w", err)
	}
	if count <= 0 {
		return fmt.Errorf("osz2: invalid file count %d", count)
	}
	if int64(count) > int64(len(encryptedInfo))/33 {
		return fmt.Errorf("osz2: file count %d exceeds the file-info table", count)
	}

	fileInfoHash := computeOszHash(encryptedInfo, int(count)*4, 0xd1)
	if fileInfoHash != r.info.FileInfoHash {
		return errors.New("osz2: file-info hash mismatch")
	}

	var currentOffset int32
	if err := binary.Read(decoder, binary.LittleEndian, &currentOffset); err != nil {
		return fmt.Errorf("osz2: read first file offset: %w", err)
	}
	if currentOffset != 0 {
		return fmt.Errorf("osz2: invalid first file offset %d", currentOffset)
	}

	fileDataSize := int32(r.size - r.dataBase)
	for i := range count {
		name, err := readString(decoder)
		if err != nil {
			return fmt.Errorf("osz2: read entry name: %w", err)
		}
		name, err = canonicalEntryPath(name) // reject path traversal etc.
		if err != nil {
			return err
		}
		if _, exists := r.entries[name]; exists {
			return fmt.Errorf("osz2: duplicate entry %q", name)
		}

		entry := &Entry{
			path: name,
		}
		if _, err := io.ReadFull(decoder, entry.hash[:]); err != nil {
			return fmt.Errorf("osz2: read hash for %q: %w", name, err)
		}

		var created, modified int64
		if err := binary.Read(decoder, binary.LittleEndian, &created); err != nil {
			return fmt.Errorf("osz2: read creation time for %q: %w", name, err)
		}
		if err := binary.Read(decoder, binary.LittleEndian, &modified); err != nil {
			return fmt.Errorf("osz2: read modification time for %q: %w", name, err)
		}

		entry.createdAt = convertFromDotNetBinary(created)
		entry.modifiedAt = convertFromDotNetBinary(modified)

		nextOffset := fileDataSize
		if i+1 < count {
			if err := binary.Read(decoder, binary.LittleEndian, &nextOffset); err != nil {
				return fmt.Errorf("osz2: read next offset for %q: %w", name, err)
			}
		}
		if nextOffset < currentOffset || nextOffset > fileDataSize || nextOffset-currentOffset < 4 {
			return fmt.Errorf("osz2: invalid data span for %q", name)
		}

		entry.size = int64(nextOffset-currentOffset) - 4
		entry.dataOffset = r.dataBase + int64(currentOffset) + 4
		actualLength, err := readEncryptedFrameLength(r.source, r.dataBase+int64(currentOffset), r.key)
		if err != nil {
			return fmt.Errorf("osz2: read content length for %q: %w", name, err)
		}
		if actualLength != entry.size {
			return fmt.Errorf("osz2: content length for %q is %d, file table says %d", name, actualLength, entry.size)
		}

		r.entries[name] = entry
		currentOffset = nextOffset
	}
	return r.applyBeatmapIDs(beatmapIDs)
}

func (r *Reader) applyBeatmapIDs(beatmapIDs map[string]int32) error {
	for name, id := range beatmapIDs {
		entry, exists := r.entries[name]
		if !exists || !entry.isBeatmapFile() {
			return fmt.Errorf("osz2: beatmap mapping references invalid entry %q", name)
		}
		if id != -1 {
			entry.beatmapID = id
			entry.hasBeatmapID = true
			r.beatmaps[id] = entry
		}
	}

	// Each beatmap is required to be assigned an ID, even if its just -1
	for name, entry := range r.entries {
		if entry.isBeatmapFile() {
			if _, exists := beatmapIDs[name]; !exists {
				return fmt.Errorf("osz2: beatmap entry %q has no ID mapping", name)
			}
		}
	}
	return nil
}

func readEncryptedFrameLength(source io.ReaderAt, offset int64, key [md5.Size]byte) (int64, error) {
	var encrypted [4]byte
	if _, err := source.ReadAt(encrypted[:], offset); err != nil {
		return 0, err
	}
	crypto.NewXXTEA(bytesToUint32s(key[:])).Decrypt(encrypted[:], 0, len(encrypted))
	return int64(binary.LittleEndian.Uint32(encrypted[:])), nil
}

func canonicalEntryPath(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Clean(name)
	if name == "." || !fs.ValidPath(name) {
		return "", fmt.Errorf("osz2: invalid entry path %q", name)
	}
	return name, nil
}

func remainingBytes(stream io.ReadSeeker) (int64, error) {
	current, err := stream.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	end, err := stream.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	if _, err := stream.Seek(current, io.SeekStart); err != nil {
		return 0, err
	}
	if end < current {
		return 0, errors.New("osz2: invalid package stream position")
	}
	return end - current, nil
}
