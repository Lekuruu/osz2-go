package osz2

import (
	"io"
)

// XXTEAReader provides streaming XXTEA decryption matching C# XXTeaStream behavior
type XXTEAReader struct {
	reader io.Reader
	xxtea  *XXTEA
}

// NewXXTEAReader creates a new XXTEAReader
func NewXXTEAReader(reader io.Reader, key []uint32) *XXTEAReader {
	return &XXTEAReader{
		reader: reader,
		xxtea:  NewXXTEA(key),
	}
}

// Read reads data from the underlying reader and decrypts it
// This matches the C# XXTeaStream.Read behavior exactly
func (x *XXTEAReader) Read(p []byte) (n int, err error) {
	// Read from underlying reader
	bytesRead, err := x.reader.Read(p)
	if err != nil && bytesRead == 0 {
		return 0, err
	}

	// Decrypt the data that was read
	x.xxtea.Decrypt(p, 0, bytesRead)

	return bytesRead, err
}

// ReadByte reads a single byte
func (x *XXTEAReader) ReadByte() (byte, error) {
	b := make([]byte, 1)
	_, err := x.Read(b)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}
