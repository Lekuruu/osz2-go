package osz2

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
)

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
