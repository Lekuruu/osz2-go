package osz2

import (
	"encoding/binary"
	"io"
	"math"
)

// Osz2Reader provides decryption for osz2 file contents
type Osz2Reader struct {
	reader     io.ReadSeeker
	offset     int
	length     int
	position   int
	skipBuffer []byte
	xxtea      *XXTEA
}

// NewOsz2Reader creates a new Osz2Reader
func NewOsz2Reader(reader io.ReadSeeker, offset int, key []byte) (*Osz2Reader, error) {
	// Read encrypted length
	encryptedLength := make([]byte, 4)
	reader.Seek(int64(offset), io.SeekStart)
	if _, err := reader.Read(encryptedLength); err != nil {
		return nil, err
	}

	// Convert key to uint32 array
	keyBuffer := bytesToUint32Array(key)
	xxtea := NewXXTEA(keyBuffer)

	// Decrypt the length
	xxtea.Decrypt(encryptedLength, 0, 4)

	// Extract length
	length := int(binary.LittleEndian.Uint32(encryptedLength))

	return &Osz2Reader{
		reader:     reader,
		offset:     offset + 4, // Skip the encrypted length
		length:     length,
		position:   offset + 4,
		skipBuffer: make([]byte, 64),
		xxtea:      xxtea,
	}, nil
}

// Position returns the current position in the stream
func (osz2 *Osz2Reader) Position() int {
	return osz2.position - osz2.offset
}

// Read reads data from the osz2 stream
func (osz2 *Osz2Reader) Read(buffer []byte) (int, error) {
	count := len(buffer)
	offset := 0

	// Adjust count if it would exceed the stream length
	if osz2.Position()+count > osz2.length {
		count = osz2.length - osz2.Position()
	}

	if count == 0 {
		return 0, io.EOF
	}

	localPosition := osz2.Position()
	seekablePosition := localPosition & ^0x3F
	skipOffset := localPosition % 64
	seekableBytes := count - (64 - skipOffset)

	var endLeftOver int
	var seekableEnd int

	// If we're not out of bounds
	if seekableBytes > 0 {
		// Calculate end of buffer
		seekableEnd = (localPosition + count) & ^0x3F
		endLeftOver = (localPosition + count) % 64
		seekableBytes = seekableEnd - (64 - skipOffset + localPosition)

		// If we have data to read
		if seekableBytes > 0 {
			// Read data and decrypt
			osz2.reader.Seek(int64(osz2.position), io.SeekStart)
			if _, err := osz2.reader.Read(buffer[offset:]); err != nil {
				return 0, err
			}
			osz2.decrypt(buffer, 64-skipOffset+offset, seekableBytes)
		}
	}

	firstBytes := int(math.Min(64, float64(osz2.length-seekablePosition)))

	// Read data and decrypt
	osz2.reader.Seek(int64(seekablePosition+osz2.offset), io.SeekStart)
	if _, err := osz2.reader.Read(osz2.skipBuffer[:firstBytes]); err != nil {
		return 0, err
	}
	osz2.decrypt(osz2.skipBuffer, 0, firstBytes)

	copyLen := int(math.Min(float64(64-skipOffset), float64(count)))
	copy(buffer[offset:], osz2.skipBuffer[skipOffset:skipOffset+copyLen])

	if endLeftOver > 0 {
		lastBytes := int(math.Min(64, float64(osz2.length-seekableEnd)))

		// Read data and decrypt
		osz2.reader.Seek(int64(seekableEnd+osz2.offset), io.SeekStart)
		if _, err := osz2.reader.Read(osz2.skipBuffer[:lastBytes]); err != nil {
			return 0, err
		}
		osz2.decrypt(osz2.skipBuffer, 0, lastBytes)

		copy(buffer[count-endLeftOver+offset:], osz2.skipBuffer[:endLeftOver])
	}

	osz2.reader.Seek(int64(osz2.position), io.SeekStart)
	osz2.seek(count, io.SeekCurrent)

	return count, nil
}

// seek moves the position in the stream
func (osz2 *Osz2Reader) seek(offset int, whence int) int {
	switch whence {
	case io.SeekStart:
		if offset >= 0 {
			osz2.position = int(math.Min(float64(offset), float64(osz2.length))) + osz2.offset
		}
	case io.SeekCurrent:
		if osz2.Position()+offset >= 0 {
			newPos := osz2.position + offset - osz2.offset
			osz2.position = int(math.Min(float64(newPos), float64(osz2.length))) + osz2.offset
		}
	case io.SeekEnd:
		if osz2.length+offset >= 0 {
			osz2.position = osz2.length + offset + osz2.offset
		}
	}
	osz2.reader.Seek(int64(osz2.position), io.SeekStart)
	return osz2.Position()
}

// decrypt decrypts data using XXTEA
func (osz2 *Osz2Reader) decrypt(buffer []byte, start, count int) {
	osz2.xxtea.Decrypt(buffer, start, count)
}
