package osz2

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Save exports and writes package bytes to disk
func (p *Package) Save(path string) error {
	data, err := p.Export()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Export exports the current package as osz2/osf2 bytes
func (p *Package) Export() ([]byte, error) {
	if len(p.FileInfos) == 0 {
		return nil, errors.New("cannot export an empty package")
	}

	key, err := p.KeyType.Generate(p.Metadata)
	if err != nil {
		return nil, err
	}
	p.key = key
	keyArray := bytesToUint32Array(key)

	files := p.prepareExportFiles()
	if len(files) == 0 {
		return nil, errors.New("cannot export package without file content")
	}

	sort.Slice(files, func(i, j int) bool {
		iVideo := files[i].IsVideo()
		jVideo := files[j].IsVideo()
		if iVideo != jVideo {
			return !iVideo
		}
		return files[i].FileName < files[j].FileName
	})

	if err := p.processVideoMetadata(files); err != nil {
		return nil, err
	}
	p.updateOffsets(files)

	fileData := writeAndEncryptFileData(files, keyArray)
	fileInfo := writeAndEncryptFileInfo(files, keyArray)

	hashInfo := computeOszHash(fileInfo, len(files)*4, 0xD1)
	videoOffset, hasVideoOffset := parseMetadataInt(p.Metadata, VideoDataOffset)
	videoLength, hasVideoLength := parseMetadataInt(p.Metadata, VideoDataLength)

	var bodyHash []byte
	if hasVideoOffset && hasVideoLength {
		bodyHash = computeBodyHash(fileData, &videoOffset, &videoLength)
	} else {
		bodyHash = computeBodyHash(fileData, nil, nil)
	}

	metaData := p.writeMetadata()
	hashMeta := computeOszHash(metaData, len(p.Metadata)*3, 0xA7)

	iv := p.IV
	if len(iv) != 16 {
		iv = make([]byte, 16)
	}
	if bytes.Equal(iv, make([]byte, 16)) {
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}
	}
	encodedIV := make([]byte, 16)
	for i := 0; i < 16; i++ {
		encodedIV[i] = iv[i] ^ bodyHash[i]
	}

	var output bytes.Buffer
	output.Write([]byte{0xEC, 0x48, 0x4F})
	output.WriteByte(p.Version)
	output.Write(encodedIV)
	output.Write(hashMeta)
	output.Write(hashInfo)
	output.Write(bodyHash)
	output.Write(metaData)

	beatmapFiles := make([]*FileInfo, 0)
	for _, file := range files {
		if file.IsBeatmap() {
			beatmapFiles = append(beatmapFiles, file)
		}
	}
	if err := binary.Write(&output, binary.LittleEndian, int32(len(beatmapFiles))); err != nil {
		return nil, err
	}
	for _, file := range beatmapFiles {
		if err := writeString(&output, file.FileName); err != nil {
			return nil, err
		}
		if err := binary.Write(&output, binary.LittleEndian, file.BeatmapID); err != nil {
			return nil, err
		}
	}

	magic := make([]byte, len(knownPlain))
	copy(magic, knownPlain)
	xtea := NewXTEA(keyArray)
	xtea.Encrypt(magic, 0, len(magic))
	output.Write(magic)

	encodedLength := len(fileInfo)
	for i := 0; i < 16; i += 2 {
		encodedLength += int(hashInfo[i]) | (int(hashInfo[i+1]) << 17)
	}
	if err := binary.Write(&output, binary.LittleEndian, int32(encodedLength)); err != nil {
		return nil, err
	}
	output.Write(fileInfo)
	output.Write(fileData)

	p.MetaDataHash = hashMeta
	p.FileInfoHash = hashInfo
	p.FullBodyHash = bodyHash
	p.IV = iv
	return output.Bytes(), nil
}

func (p *Package) prepareExportFiles() []*FileInfo {
	files := make([]*FileInfo, 0, len(p.FileInfos))
	now := time.Now().UTC()

	for name, info := range p.FileInfos {
		if info == nil {
			continue
		}
		if info.FileName == "" {
			info.FileName = name
		}
		if info.Content == nil {
			continue
		}

		if info.DateCreated.IsZero() {
			info.DateCreated = now
		}
		if info.DateModified.IsZero() {
			info.DateModified = now
		}
		info.DateCreated = info.DateCreated.UTC()
		info.DateModified = info.DateModified.UTC()
		info.Size = int32(len(info.Content) + 4)
		info.Hash = ComputeHashBytesRaw(info.Content)

		if beatmapID, ok := p.FileNames[name]; ok {
			info.BeatmapID = beatmapID
		} else if info.IsBeatmap() && info.BeatmapID == 0 {
			info.BeatmapID = -1
		}

		files = append(files, info)
	}

	return files
}

func (p *Package) processVideoMetadata(files []*FileInfo) error {
	offset := 0
	for _, file := range files {
		if !file.IsVideo() {
			offset += 4 + len(file.Content)
			continue
		}
		if len(file.Content) < 1024 {
			return errors.New("video needs to be at least 1024 bytes")
		}

		dataLength := len(file.Content)
		footStart := (dataLength / 2) - ((dataLength / 2) % 16) - 512 + 16
		footData := file.Content[footStart : footStart+1024]
		videoHash := strings.ToUpper(fmt.Sprintf("%x", md5.Sum(footData)))

		p.Metadata[VideoDataOffset] = strconv.Itoa(offset)
		p.Metadata[VideoDataLength] = strconv.Itoa(dataLength)
		p.Metadata[VideoHash] = videoHash
		break
	}
	return nil
}

func (p *Package) updateOffsets(files []*FileInfo) {
	offset := int32(0)
	for _, file := range files {
		file.Offset = offset
		offset += int32(4 + len(file.Content))
		file.Size = int32(4 + len(file.Content))
		file.Hash = ComputeHashBytesRaw(file.Content)
	}
}

func writeAndEncryptFileData(files []*FileInfo, key []uint32) []byte {
	writer := NewXXTEAWriter(key)
	for _, file := range files {
		binary.Write(writer, binary.LittleEndian, int32(len(file.Content)))
		writer.Write(file.Content)
	}
	return writer.Bytes()
}

func writeAndEncryptFileInfo(files []*FileInfo, key []uint32) []byte {
	writer := NewXXTEAWriter(key)
	binary.Write(writer, binary.LittleEndian, int32(len(files)))
	if len(files) > 0 {
		binary.Write(writer, binary.LittleEndian, files[0].Offset)
	}

	for i, file := range files {
		writeString(writer, file.FileName)
		if len(file.Hash) >= 16 {
			writer.Write(file.Hash[:16])
		} else {
			h := make([]byte, 16)
			copy(h, file.Hash)
			writer.Write(h)
		}

		binary.Write(writer, binary.LittleEndian, datetimeToDotNetBinary(file.DateCreated))
		binary.Write(writer, binary.LittleEndian, datetimeToDotNetBinary(file.DateModified))
		if i < len(files)-1 {
			binary.Write(writer, binary.LittleEndian, files[i+1].Offset)
		}
	}
	return writer.Bytes()
}

func (p *Package) writeMetadata() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, int32(len(p.Metadata)))
	for metaType, value := range p.Metadata {
		binary.Write(&buf, binary.LittleEndian, int16(metaType))
		writeString(&buf, value)
	}
	return buf.Bytes()
}
