package osz2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// Package represents an osz2 package
type Package struct {
	// Metadata contains .osu metadata (e.g Artist, Difficulty, etc..)
	Metadata map[MetaType]string

	// FileInfos contains .osu file info (e.g FileName, Hash, Size etc..)
	FileInfos map[string]*FileInfo

	// Files contains osz2 file contents
	Files map[string][]byte

	// FileNames maps filename to beatmap id
	FileNames map[string]int32

	// FileIDs maps beatmap id to filename
	FileIDs map[int32]string

	// Hashes
	MetaDataHash []byte
	FileInfoHash []byte
	FullBodyHash []byte

	// Key for XTEA algorithm
	key []byte

	// Need decrypt only metadata?
	metadataOnly bool
}

// NewPackage creates a new osz2 package from a reader
func NewPackage(r io.ReadSeeker, metadataOnly bool) (*Package, error) {
	p := &Package{
		Metadata:     make(map[MetaType]string),
		FileInfos:    make(map[string]*FileInfo),
		Files:        make(map[string][]byte),
		FileNames:    make(map[string]int32),
		FileIDs:      make(map[int32]string),
		metadataOnly: metadataOnly,
	}

	err := p.read(r)
	if err != nil {
		return nil, err
	}

	return p, nil
}

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

	// Skip unused version byte
	r.Seek(1, io.SeekCurrent)

	// Skip unused IV
	r.Seek(16, io.SeekCurrent)

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

	// Generate seed using metadata
	creator, ok_creator := p.Metadata[Creator]
	beatmapSetID, ok_setID := p.Metadata[BeatmapSetID]

	if !ok_creator || !ok_setID {
		return errors.New("missing required metadata for key generation")
	}

	seed := creator + "yhxyfjo5" + beatmapSetID
	p.key = ComputeHashBytesRaw([]byte(seed))

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

// readString reads a .NET style string (length-prefixed)
func readString(r io.Reader) (string, error) {
	// Read length (7-bit encoded)
	length, err := read7BitEncodedInt(r)
	if err != nil {
		return "", err
	}

	if length == 0 {
		return "", nil
	}

	data := make([]byte, length)
	if _, err := r.Read(data); err != nil {
		return "", err
	}

	return string(data), nil
}

// readStringFromBuffer reads a string from a byte buffer
func readStringFromBuffer(r io.Reader) (string, error) {
	// Read length (7-bit encoded)
	length, err := read7BitEncodedIntFromBuffer(r)
	if err != nil {
		return "", err
	}

	if length == 0 {
		return "", nil
	}

	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// readStringFromStream reads a string from a stream reader
func readStringFromStream(stream io.Reader) (string, error) {
	// Read length (7-bit encoded)
	length, err := read7BitEncodedIntFromStream(stream)
	if err != nil {
		return "", err
	}

	if length == 0 {
		return "", nil
	}

	data := make([]byte, length)
	if _, err := stream.Read(data); err != nil {
		return "", err
	}

	return string(data), nil
}

// read7BitEncodedIntFromStream reads a 7-bit encoded int from a stream
func read7BitEncodedIntFromStream(stream io.Reader) (int32, error) {
	var result int32
	var shift uint
	for {
		var b byte
		if err := binary.Read(stream, binary.LittleEndian, &b); err != nil {
			return 0, err
		}

		result |= int32(b&0x7F) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7

		// Protect against malformed data
		if shift >= 32 {
			return 0, errors.New("7-bit encoded int is too large")
		}
	}
	return result, nil
}

// writeStringToBuffer writes a string to buffer in .NET format
func writeStringToBuffer(buf *bytes.Buffer, s string) {
	write7BitEncodedInt(buf, len(s))
	buf.WriteString(s)
}

// read7BitEncodedInt reads a 7-bit encoded integer (.NET style)
func read7BitEncodedInt(r io.Reader) (int, error) {
	var result int
	var shift uint

	for {
		b := make([]byte, 1)
		if _, err := r.Read(b); err != nil {
			return 0, err
		}

		result |= int(b[0]&0x7F) << shift
		if b[0]&0x80 == 0 {
			break
		}
		shift += 7
	}

	return result, nil
}

// read7BitEncodedIntFromBuffer reads a 7-bit encoded integer from buffer
func read7BitEncodedIntFromBuffer(r io.Reader) (int, error) {
	var result int
	var shift uint
	b := make([]byte, 1)

	for {
		_, err := r.Read(b)
		if err != nil {
			return 0, err
		}

		result |= int(b[0]&0x7F) << shift
		if b[0]&0x80 == 0 {
			break
		}
		shift += 7
	}

	return result, nil
}

// write7BitEncodedInt writes a 7-bit encoded integer
func write7BitEncodedInt(buf *bytes.Buffer, value int) {
	for value >= 0x80 {
		buf.WriteByte(byte(value | 0x80))
		value >>= 7
	}
	buf.WriteByte(byte(value))
}

// bytesToUint32Array converts byte array to uint32 array
func bytesToUint32Array(data []byte) []uint32 {
	result := make([]uint32, len(data)/4)
	for i := 0; i < len(result); i++ {
		result[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	return result
}

// computeOszHash computes MD5 hash of .osz parts
func computeOszHash(buffer []byte, pos int, swap byte) []byte {
	// Make a copy to avoid modifying the original
	buf := make([]byte, len(buffer))
	copy(buf, buffer)

	// Ensure pos is within bounds
	if pos >= len(buf) {
		// If position is out of bounds, just compute hash without swapping
		hash := ComputeHashBytesRaw(buf)

		// Swap bytes as in C# implementation
		for i := 0; i < 8; i++ {
			tmp := hash[i]
			hash[i] = hash[i+8]
			hash[i+8] = tmp
		}

		hash[5] ^= 0x2d
		return hash
	}

	buf[pos] ^= swap
	hash := ComputeHashBytesRaw(buf)
	buf[pos] ^= swap // restore original

	// Swap bytes as in C# implementation
	for i := 0; i < 8; i++ {
		tmp := hash[i]
		hash[i] = hash[i+8]
		hash[i+8] = tmp
	}

	hash[5] ^= 0x2d
	return hash
}

// convertFromDotNetBinary converts a .NET DateTime.ToBinary() value to a Go time.Time
func convertFromDotNetBinary(binary int64) time.Time {
	// .NET DateTime ticks are 100-nanosecond intervals since January 1, 0001
	// Unix epoch is January 1, 1970, which is 621,355,968,000,000,000 ticks after January 1, 0001
	const dotNetToUnixEpochTicks = 621355968000000000

	// Extract the ticks (lower 62 bits) and ignore the Kind flags (upper 2 bits)
	ticks := binary & 0x3FFFFFFFFFFFFFFF

	// Convert to Unix timestamp (nanoseconds)
	unixNanos := (ticks - dotNetToUnixEpochTicks) * 100

	return time.Unix(0, unixNanos).UTC()
}
