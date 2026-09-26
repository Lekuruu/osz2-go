package osz2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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
	if err := validateReadLength(r, length); err != nil {
		return "", err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
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
	if err := validateReadLength(r, length); err != nil {
		return "", err
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
	var result uint32
	b := make([]byte, 1)
	for i := range 5 {
		if _, err := io.ReadFull(r, b); err != nil {
			return 0, err
		}
		if i == 4 && b[0] > 0x07 {
			return 0, errors.New("7-bit encoded integer overflow")
		}

		result |= uint32(b[0]&0x7F) << (7 * i)
		if b[0]&0x80 == 0 {
			return int(result), nil
		}
	}
	return 0, errors.New("7-bit encoded integer overflow")
}

func read7BitEncodedIntFromBuffer(r io.Reader) (int, error) {
	return read7BitEncodedInt(r)
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
	for i := range result {
		result[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	return result
}

func computeOszHash(buffer []byte, pos int, swap byte) []byte {
	// Make a copy to avoid modifying the original
	buf := bytes.Clone(buffer)

	// Ensure pos is within bounds
	if pos >= len(buf) {
		// If position is out of bounds, just compute hash without swapping
		hash := ComputeHashBytesRaw(buf)

		for i := range 8 {
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

	for i := range 8 {
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

func convertFromDotNetBinary(value int64) time.Time {
	const (
		dotNetToUnixEpochTicks = int64(621_355_968_000_000_000)
		ticksPerSecond         = int64(10_000_000)
	)

	ticks := value & 0x3FFFFFFFFFFFFFFF
	unixTicks := ticks - dotNetToUnixEpochTicks
	seconds := unixTicks / ticksPerSecond
	nanoseconds := unixTicks % ticksPerSecond * 100

	return time.Unix(seconds, nanoseconds).UTC()
}

func datetimeToDotNetBinary(t time.Time) (int64, error) {
	const (
		dotNetToUnixEpochTicks = int64(621_355_968_000_000_000)
		ticksPerSecond         = int64(10_000_000)
	)

	if t.Year() < 1 || t.Year() > 9999 {
		return 0, fmt.Errorf("timestamp year %d is outside the .NET DateTime range", t.Year())
	}

	ticks := dotNetToUnixEpochTicks + t.Unix()*ticksPerSecond + int64(t.Nanosecond()/100)
	return ticks, nil
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

func validateReadLength(r io.Reader, length int) error {
	type HasLength interface{ Len() int } // yea idk what else to call this

	// Try to determine length from reader Len() if provided
	if remainingReader, ok := r.(HasLength); ok {
		remaining := remainingReader.Len()
		if remaining >= 0 && length > remaining {
			return fmt.Errorf("declared length %d exceeds remaining data %d", length, remaining)
		}
		return nil
	}

	// Try to determine length from seeker
	seeker, ok := r.(io.Seeker)
	if !ok {
		return nil
	}

	current, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	end, err := seeker.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := seeker.Seek(current, io.SeekStart); err != nil {
		return err
	}
	if end < current || int64(length) > end-current {
		return fmt.Errorf("declared length %d exceeds remaining data %d", length, end-current)
	}
	return nil
}
