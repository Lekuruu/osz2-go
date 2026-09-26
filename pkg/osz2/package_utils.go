package osz2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

const (
	dotNetToUnixEpochTicks = int64(621_355_968_000_000_000)
	ticksPerSecond         = int64(10_000_000)
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
	if err := write7BitEncodedInt(w, len(s)); err != nil {
		return err
	}
	if len(s) == 0 {
		return nil
	}
	_, err := writeFull(w, []byte(s))
	return err
}

func writeFull(w io.Writer, p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n, err := w.Write(p)
		if n < 0 || n > len(p) {
			return written, errors.New("writer returned an invalid byte count")
		}

		written += n
		p = p[n:]
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func read7BitEncodedInt(r io.Reader) (int, error) {
	var result uint32
	var encoded [1]byte
	for i := range 5 {
		if _, err := io.ReadFull(r, encoded[:]); err != nil {
			return 0, err
		}
		if i == 4 && encoded[0] > 0x07 {
			return 0, errors.New("7-bit encoded integer overflow")
		}

		result |= uint32(encoded[0]&0x7f) << (7 * i)
		if encoded[0]&0x80 == 0 {
			return int(result), nil
		}
	}
	return 0, errors.New("7-bit encoded integer overflow")
}

func write7BitEncodedInt(w io.Writer, value int) error {
	var encoded [1]byte
	for value >= 0x80 {
		encoded[0] = byte(value | 0x80)
		if _, err := writeFull(w, encoded[:]); err != nil {
			return err
		}
		value >>= 7
	}

	encoded[0] = byte(value)
	_, err := writeFull(w, encoded[:])
	return err
}

func bytesToUint32s(data []byte) []uint32 {
	result := make([]uint32, len(data)/4)
	for i := range result {
		result[i] = binary.LittleEndian.Uint32(data[i*4:])
	}
	return result
}

func convertFromDotNetBinary(value int64) time.Time {
	ticks := value & 0x3FFFFFFFFFFFFFFF
	unixTicks := ticks - dotNetToUnixEpochTicks
	seconds := unixTicks / ticksPerSecond
	nanoseconds := unixTicks % ticksPerSecond * 100

	return time.Unix(seconds, nanoseconds).UTC()
}

func datetimeToDotNetBinary(t time.Time) (int64, error) {
	if t.Year() < 1 || t.Year() > 9999 {
		return 0, fmt.Errorf("timestamp year %d is outside the .NET DateTime range", t.Year())
	}

	ticks := dotNetToUnixEpochTicks + t.Unix()*ticksPerSecond + int64(t.Nanosecond()/100)
	return ticks, nil
}

func parseMetadataInt(metadata Metadata, key MetaType) (int64, bool) {
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func bodyHashExclusion(metadata Metadata, total int64) (start, length int64) {
	startValue, hasStart := parseMetadataInt(metadata, VideoDataOffset)
	lengthValue, hasLength := parseMetadataInt(metadata, VideoDataLength)
	if !hasStart || !hasLength || startValue < 0 || lengthValue < 0 {
		return -1, 0
	}

	start = startValue
	length = lengthValue
	if start > total || length > total-start {
		return -1, 0
	}
	return start, length
}

func validateReadLength(r io.Reader, length int) error {
	if remainingReader, ok := r.(interface{ Len() int }); ok {
		remaining := remainingReader.Len()
		if remaining >= 0 && length > remaining {
			return fmt.Errorf("declared length %d exceeds remaining data %d", length, remaining)
		}
		return nil
	}

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
