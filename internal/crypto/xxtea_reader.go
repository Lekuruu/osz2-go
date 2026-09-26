package crypto

import (
	"io"
)

type XXTEAReader struct {
	reader io.Reader
	xxtea  *XXTEA
}

// NewXXTEAReader creates a reader for the format's write-framed encryption.
func NewXXTEAReader(reader io.Reader, key []uint32) *XXTEAReader {
	return &XXTEAReader{
		reader: reader,
		xxtea:  NewXXTEA(key),
	}
}

// Read decrypts one frame read from the underlying reader.
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

// Len returns the number of encrypted bytes still available when the underlying reader provides it
func (x *XXTEAReader) Len() int {
	if reader, ok := x.reader.(interface{ Len() int }); ok {
		return reader.Len()
	}
	return -1
}
