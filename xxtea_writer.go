package osz2

import "bytes"

// XXTEAWriter provides streaming XXTEA encryption
type XXTEAWriter struct {
	buf   bytes.Buffer
	xxtea *XXTEA
}

// NewXXTEAWriter creates a new XXTEA writer with the provided key
func NewXXTEAWriter(key []uint32) *XXTEAWriter {
	return &XXTEAWriter{xxtea: NewXXTEA(key)}
}

// Write encrypts p and appends the encrypted bytes to the internal buffer
func (w *XXTEAWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	encrypted := append([]byte(nil), p...)
	w.xxtea.Encrypt(encrypted, 0, len(encrypted))
	return w.buf.Write(encrypted)
}

// Bytes returns the accumulated encrypted output bytes
func (w *XXTEAWriter) Bytes() []byte {
	return w.buf.Bytes()
}

// Reset clears the internal buffer while keeping the same cryptor instance
func (w *XXTEAWriter) Reset() {
	w.buf.Reset()
}
