package osz2

import (
	"bytes"
	"encoding/binary"
	"io"
	"strconv"
	"strings"
	"time"
)

func readString(r io.Reader) (string, error) {
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

func writeString(w io.Writer, s string) error {
	var length bytes.Buffer
	write7BitEncodedInt(&length, len(s))
	if _, err := w.Write(length.Bytes()); err != nil {
		return err
	}
	if len(s) == 0 {
		return nil
	}
	_, err := io.WriteString(w, s)
	return err
}

func readStringFromBuffer(r io.Reader) (string, error) {
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

func writeStringToBuffer(buf *bytes.Buffer, s string) {
	write7BitEncodedInt(buf, len(s))
	buf.WriteString(s)
}

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

func write7BitEncodedInt(buf *bytes.Buffer, value int) {
	for value >= 0x80 {
		buf.WriteByte(byte(value | 0x80))
		value >>= 7
	}
	buf.WriteByte(byte(value))
}

func bytesToUint32Array(data []byte) []uint32 {
	result := make([]uint32, len(data)/4)
	for i := 0; i < len(result); i++ {
		result[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	return result
}

func computeOszHash(buffer []byte, pos int, swap byte) []byte {
	// Make a copy to avoid modifying the original
	buf := make([]byte, len(buffer))
	copy(buf, buffer)

	// Ensure pos is within bounds
	if pos >= len(buf) {
		// If position is out of bounds, just compute hash without swapping
		hash := ComputeHashBytesRaw(buf)

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

	for i := 0; i < 8; i++ {
		tmp := hash[i]
		hash[i] = hash[i+8]
		hash[i+8] = tmp
	}

	hash[5] ^= 0x2d
	return hash
}

func computeBodyHash(data []byte, videoOffset, videoLength *int) []byte {
	toHash := data
	if videoOffset != nil && videoLength != nil {
		start := *videoOffset
		length := *videoLength
		if start >= 0 && length >= 0 && start+length <= len(data) {
			filtered := make([]byte, 0, len(data)-length)
			filtered = append(filtered, data[:start]...)
			filtered = append(filtered, data[start+length:]...)
			toHash = filtered
		}
	}
	pos := len(toHash) / 2
	return computeOszHash(toHash, pos, 0x9F)
}

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

func datetimeToDotNetBinary(t time.Time) int64 {
	t = t.UTC()
	base := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	delta := t.Sub(base)
	ticks := delta.Nanoseconds() / 100
	return ticks & 0x3FFFFFFFFFFFFFFF
}

func parseMetadataInt(metadata map[MetaType]string, key MetaType) (int, bool) {
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func sanitizeFilename(filename string) string {
	replacer := strings.NewReplacer("<", "", ">", "", ":", "", "\"", "", "|", "", "?", "", "*", "")
	cleaned := replacer.Replace(filename)
	for strings.Contains(cleaned, "../") || strings.Contains(cleaned, "..\\") {
		cleaned = strings.ReplaceAll(cleaned, "../", "")
		cleaned = strings.ReplaceAll(cleaned, "..\\", "")
	}
	return cleaned
}
