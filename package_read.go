package osz2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

func (p *Package) read(r io.ReadSeeker) error {
	// Read identifier (magic number)
	identifier := make([]byte, 3)
	if _, err := io.ReadFull(r, identifier); err != nil {
		return err
	}

	// Check if given .osz2 package is valid
	if len(identifier) < 3 ||
		identifier[0] != 0xEC ||
		identifier[1] != 0x48 ||
		identifier[2] != 0x4F {
		return errors.New("file is not valid .osz2 package")
	}

	// Read unused version byte
	version := make([]byte, 1)
	if _, err := io.ReadFull(r, version); err != nil {
		return err
	}
	p.Version = version[0]

	// Read IV
	if _, err := io.ReadFull(r, p.IV); err != nil {
		return err
	}

	// Read hashes of .osu parts
	p.MetaDataHash = make([]byte, 16)
	p.FileInfoHash = make([]byte, 16)
	p.FullBodyHash = make([]byte, 16)

	if _, err := io.ReadFull(r, p.MetaDataHash); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, p.FileInfoHash); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, p.FullBodyHash); err != nil {
		return err
	}

	// Read metadata block
	if err := p.readMetadata(r); err != nil {
		return err
	}

	// Read file names mapping
	if err := p.readFileNames(r); err != nil {
		return err
	}

	// Generate key using selected key type
	var err error
	p.key, err = p.KeyType.Generate(p.Metadata)
	if err != nil {
		return err
	}

	if !p.metadataOnly {
		return p.readFiles(r)
	}

	return nil
}

func (p *Package) readMetadata(r io.ReadSeeker) error {
	var count int32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
	}
	if count < 0 {
		return fmt.Errorf("invalid metadata count: %d", count)
	}

	// Buffer to store data for hash verification
	var buf bytes.Buffer
	buf.WriteByte(byte(count))
	buf.WriteByte(byte(count >> 8))
	buf.WriteByte(byte(count >> 16))
	buf.WriteByte(byte(count >> 24))

	// Read metadata
	for range count {
		var metaType int16
		if err := binary.Read(r, binary.LittleEndian, &metaType); err != nil {
			return err
		}

		metaValue, err := readString(r)
		if err != nil {
			return err
		}

		// Store metadata if it's a valid type
		p.Metadata[MetaType(metaType)] = metaValue

		// Write to buffer for hash verification
		buf.WriteByte(byte(metaType))
		buf.WriteByte(byte(metaType >> 8))
		writeStringToBuffer(&buf, metaValue)
	}

	// Verify metadata hash
	hash := computeOszHash(buf.Bytes(), int(count)*3, 0xa7)
	if !bytes.Equal(hash, p.MetaDataHash) {
		return errors.New("metadata hash mismatch")
	}

	return nil
}

func (p *Package) readFileNames(r io.ReadSeeker) error {
	var mapsCount int32
	if err := binary.Read(r, binary.LittleEndian, &mapsCount); err != nil {
		return err
	}
	if mapsCount < 0 {
		return fmt.Errorf("invalid beatmap count: %d", mapsCount)
	}

	// Read all maps in .osz2 and add them to dictionaries
	for range mapsCount {
		fileName, err := readString(r)
		if err != nil {
			return err
		}

		var beatmapID int32
		if err := binary.Read(r, binary.LittleEndian, &beatmapID); err != nil {
			return err
		}
		if _, exists := p.FileNames[fileName]; exists {
			return fmt.Errorf("duplicate beatmap filename: %q", fileName)
		}
		if beatmapID != -1 {
			if owner, exists := p.FileIDs[beatmapID]; exists {
				return fmt.Errorf("beatmap ID %d is assigned to both %q and %q", beatmapID, owner, fileName)
			}
			p.FileIDs[beatmapID] = fileName
		}

		p.FileNames[fileName] = beatmapID
		if info, ok := p.FileInfos[fileName]; ok && info != nil {
			info.BeatmapID = beatmapID
		}
	}

	return nil
}

func (p *Package) readFiles(r io.ReadSeeker) error {
	// Convert key to uint32 array for XTEA
	key := bytesToUint32Array(p.key)

	// Create XTEA for reading magic bytes
	xtea := NewXTEA(key)

	// Read and decrypt magic encrypted bytes
	plain := make([]byte, 64)
	if _, err := io.ReadFull(r, plain); err != nil {
		return err
	}
	xtea.Decrypt(plain, 0, 64)

	if !bytes.Equal(plain, knownPlain) {
		return errors.New("invalid encryption key")
	}

	// Read encrypted length
	var length int32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return err
	}

	// Decode length by encrypted length
	for i := 0; i < 16; i += 2 {
		length -= int32(p.FileInfoHash[i]) | (int32(p.FileInfoHash[i+1]) << 17)
	}
	if length < 0 {
		return fmt.Errorf("invalid file info length: %d", length)
	}

	fileInfoStart, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	totalSize, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := r.Seek(fileInfoStart, io.SeekStart); err != nil {
		return err
	}
	if totalSize < fileInfoStart || int64(length) > totalSize-fileInfoStart {
		return fmt.Errorf("file info length %d exceeds remaining package data", length)
	}

	// Read all .osu files info
	fileInfo := make([]byte, length)
	if _, err := io.ReadFull(r, fileInfo); err != nil {
		return err
	}

	// Get file start offset
	fileOffset := fileInfoStart + int64(length)

	// Create an XXTEA reader from the encrypted fileInfo bytes
	// This matches the C# approach where XXTeaStream wraps the MemoryStream
	// and decrypts incrementally as BinaryReader requests bytes
	keyArray := bytesToUint32Array(p.key)

	// Create XXTEA reader to decrypt file info
	fileInfoReader := NewXXTEAReader(bytes.NewReader(fileInfo), keyArray)

	// Parse the file info using the streaming XXTEA reader
	err = p.parseFileInfo(
		fileInfoReader, fileInfo,
		int(fileOffset), int(totalSize),
	)

	if err != nil {
		return err
	}

	// Read file contents
	return p.readFileContents(r, int(fileOffset))
}

func (p *Package) parseFileInfo(r io.Reader, encryptedFileInfo []byte, fileOffset int, totalSize int) error {
	if fileOffset < 0 || totalSize < fileOffset || totalSize-fileOffset >= 1<<31 {
		return fmt.Errorf("invalid file data bounds: offset %d, total size %d", fileOffset, totalSize)
	}
	fileDataSize := int32(totalSize - fileOffset)

	var count int32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
	}
	if count <= 0 {
		return fmt.Errorf("invalid file count: %d", count)
	}

	// Verify file info hash
	fileInfoHash := computeOszHash(encryptedFileInfo, int(count)*4, 0xd1)
	if !bytes.Equal(fileInfoHash, p.FileInfoHash) {
		return errors.New("fileInfo hash mismatch")
	}

	var currentOffset int32
	if err := binary.Read(r, binary.LittleEndian, &currentOffset); err != nil {
		return err
	}
	if currentOffset != 0 {
		return fmt.Errorf("invalid first file offset: %d", currentOffset)
	}

	for i := range count {
		fileName, err := readStringFromBuffer(r)
		if err != nil {
			return err
		}
		if _, exists := p.FileInfos[fileName]; exists {
			return fmt.Errorf("duplicate file info filename: %q", fileName)
		}

		fileHash := make([]byte, 16)
		if _, err := io.ReadFull(r, fileHash); err != nil {
			return err
		}

		var dateCreatedBinary, dateModifiedBinary int64
		if err := binary.Read(r, binary.LittleEndian, &dateCreatedBinary); err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &dateModifiedBinary); err != nil {
			return err
		}

		// Convert from .NET DateTime.ToBinary() format
		// .NET DateTime ticks are 100-nanosecond intervals since January 1, 0001
		// DateTime.ToBinary() encodes both the ticks and the Kind
		dateCreated := convertFromDotNetBinary(dateCreatedBinary)
		dateModified := convertFromDotNetBinary(dateModifiedBinary)

		var nextOffset int32
		if i+1 < count {
			if err := binary.Read(r, binary.LittleEndian, &nextOffset); err != nil {
				return err
			}
		} else {
			// For last file, calculate size differently - use total file size minus file offset
			nextOffset = fileDataSize
		}
		if nextOffset < currentOffset || nextOffset > fileDataSize {
			return fmt.Errorf("invalid offset for file %q: %d after %d", fileName, nextOffset, currentOffset)
		}

		fileLength := nextOffset - currentOffset
		if fileLength < 4 {
			return fmt.Errorf("invalid size for file %q: %d", fileName, fileLength)
		}

		info := NewFileInfo(
			fileName, currentOffset, fileLength,
			fileHash, dateCreated, dateModified,
		)
		if beatmapID, ok := p.FileNames[fileName]; ok {
			info.BeatmapID = beatmapID
		}
		p.FileInfos[fileName] = info

		// Move to next file offset
		currentOffset = nextOffset
	}

	return p.validateBeatmapMappings()
}

func (p *Package) readFileContents(r io.ReadSeeker, fileOffset int) error {
	for fileName, fileInfo := range p.FileInfos {
		content := make([]byte, fileInfo.Size-4) // -4 because of the encrypted length prefix
		_, err := readEncryptedEntryContent(r, fileOffset+int(fileInfo.Offset), p.key, content)
		if err != nil {
			return fmt.Errorf("read file %q: %w", fileName, err)
		}

		if info, ok := p.FileInfos[fileName]; ok && info != nil {
			info.Content = content
		}
	}
	return nil
}

func readEncryptedEntryLength(reader io.ReadSeeker, offset int, xxtea *XXTEA) (int, error) {
	encryptedLength := make([]byte, 4)
	if _, err := reader.Seek(int64(offset), io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(reader, encryptedLength); err != nil {
		return 0, err
	}

	xxtea.Decrypt(encryptedLength, 0, 4)
	return int(binary.LittleEndian.Uint32(encryptedLength)), nil
}

func readEncryptedEntryContent(reader io.ReadSeeker, offset int, key []byte, buffer []byte) (int, error) {
	xxtea := NewXXTEA(bytesToUint32Array(key))
	entryLength, err := readEncryptedEntryLength(reader, offset, xxtea)
	if err != nil {
		return 0, err
	}

	if entryLength != len(buffer) {
		return 0, fmt.Errorf("entry length %d does not match file table length %d", entryLength, len(buffer))
	}
	if entryLength == 0 {
		return 0, nil
	}
	if _, err := reader.Seek(int64(offset+4), io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return 0, err
	}
	xxtea.Decrypt(buffer, 0, entryLength)
	return entryLength, nil
}
