package osz2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// read reads the osz2 package data
func (p *Package) read(r io.ReadSeeker) error {
	// Read identifier (magic number)
	identifier := make([]byte, 3)
	if _, err := r.Read(identifier); err != nil {
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
	if _, err := r.Read(version); err != nil {
		return err
	}
	p.Version = version[0]

	// Read IV
	if _, err := r.Read(p.IV); err != nil {
		return err
	}

	// Read hashes of .osu parts
	p.MetaDataHash = make([]byte, 16)
	p.FileInfoHash = make([]byte, 16)
	p.FullBodyHash = make([]byte, 16)

	if _, err := r.Read(p.MetaDataHash); err != nil {
		return err
	}
	if _, err := r.Read(p.FileInfoHash); err != nil {
		return err
	}
	if _, err := r.Read(p.FullBodyHash); err != nil {
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

// readMetadata reads the metadata section
func (p *Package) readMetadata(r io.ReadSeeker) error {
	var count int32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
	}

	// Buffer to store data for hash verification
	var buf bytes.Buffer
	buf.WriteByte(byte(count))
	buf.WriteByte(byte(count >> 8))
	buf.WriteByte(byte(count >> 16))
	buf.WriteByte(byte(count >> 24))

	// Read metadata
	for i := int32(0); i < count; i++ {
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

// readFileNames reads the filename to beatmap ID mapping
func (p *Package) readFileNames(r io.ReadSeeker) error {
	var mapsCount int32
	if err := binary.Read(r, binary.LittleEndian, &mapsCount); err != nil {
		return err
	}

	// Read all maps in .osz2 and add them to dictionaries
	for i := int32(0); i < mapsCount; i++ {
		fileName, err := readString(r)
		if err != nil {
			return err
		}

		var beatmapID int32
		if err := binary.Read(r, binary.LittleEndian, &beatmapID); err != nil {
			return err
		}

		p.FileNames[fileName] = beatmapID
		p.FileIDs[beatmapID] = fileName
	}

	return nil
}

// readFiles reads the actual file contents
func (p *Package) readFiles(r io.ReadSeeker) error {
	// Convert key to uint32 array for XTEA
	key := bytesToUint32Array(p.key)

	// Create XTEA for reading magic bytes
	xtea := NewXTEA(key)

	// Read and decrypt magic encrypted bytes
	plain := make([]byte, 64)
	if _, err := r.Read(plain); err != nil {
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

	// Read all .osu files info
	fileInfo := make([]byte, length)
	if _, err := r.Read(fileInfo); err != nil {
		return err
	}

	// Get file start offset
	fileOffset, _ := r.Seek(0, io.SeekCurrent)

	// Get total file size
	currentPos, _ := r.Seek(0, io.SeekCurrent)
	totalSize, _ := r.Seek(0, io.SeekEnd)
	r.Seek(currentPos, io.SeekStart)

	// Create an XXTEA reader from the encrypted fileInfo bytes
	// This matches the C# approach where XXTeaStream wraps the MemoryStream
	// and decrypts incrementally as BinaryReader requests bytes
	keyArray := bytesToUint32Array(p.key)

	// Create XXTEA reader to decrypt file info
	fileInfoReader := NewXXTEAReader(bytes.NewReader(fileInfo), keyArray)

	// Parse the file info using the streaming XXTEA reader
	err := p.parseFileInfo(
		fileInfoReader, fileInfo,
		int(fileOffset), int(totalSize),
	)

	if err != nil {
		return err
	}

	// Read file contents
	return p.readFileContents(r, int(fileOffset))
}

// parseFileInfo parses the decrypted file info section
func (p *Package) parseFileInfo(r io.Reader, encryptedFileInfo []byte, fileOffset int, totalSize int) error {
	var count int32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
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

	for i := int32(0); i < count; i++ {
		fileName, err := readStringFromBuffer(r)
		if err != nil {
			return err
		}

		fileHash := make([]byte, 16)
		if _, err := r.Read(fileHash); err != nil {
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
			nextOffset = int32(totalSize - fileOffset)
		}

		fileLength := nextOffset - currentOffset

		p.FileInfos[fileName] = NewFileInfo(
			fileName, currentOffset, fileLength,
			fileHash, dateCreated, dateModified,
		)

		// Move to next file offset
		currentOffset = nextOffset
	}

	return nil
}

// readFileContents reads the actual file contents
func (p *Package) readFileContents(r io.ReadSeeker, fileOffset int) error {
	for fileName, fileInfo := range p.FileInfos {
		// Create Osz2Stream equivalent
		osz2Reader, err := NewOsz2Reader(r, fileOffset+int(fileInfo.Offset), p.key)
		if err != nil {
			fmt.Printf("Failed to create reader for: %s\n", fileName)
			continue
		}

		// Read file content
		content := make([]byte, fileInfo.Size-4) // -4 because of the encrypted length prefix
		_, err = osz2Reader.Read(content)
		if err != nil {
			fmt.Printf("Failed to read: %s\n", fileName)
			continue
		}

		p.Files[fileName] = content
	}

	return nil
}
