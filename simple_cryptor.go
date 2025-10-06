package osz2

// SimpleCryptor implements the simple encryption used in osz2
type SimpleCryptor struct {
	key []uint32
}

// NewSimpleCryptor creates a new SimpleCryptor
func NewSimpleCryptor(key []uint32) *SimpleCryptor {
	return &SimpleCryptor{key: key}
}

// EncryptBytes encrypts bytes in place
func (sc *SimpleCryptor) EncryptBytes(buf []byte) {
	byteKey := uint32SliceToByteSlice(sc.key)
	var prevEncrypted byte = 0

	for i := 0; i < len(buf); i++ {
		// Handle modulo properly for potentially negative values
		sum := int(buf[i]) + int(byteKey[i%16]>>2)
		buf[i] = byte((sum%256 + 256) % 256)

		buf[i] ^= rotateLeft(byteKey[15-i%16], byte((int(prevEncrypted)+len(buf)-i)%7))
		buf[i] = rotateRight(buf[i], byte((^uint32(prevEncrypted))%7))

		prevEncrypted = buf[i]
	}
}

// DecryptBytes decrypts bytes in place
func (sc *SimpleCryptor) DecryptBytes(buf []byte) {
	byteKey := uint32SliceToByteSlice(sc.key)
	var prevEncrypted byte = 0

	for i := 0; i < len(buf); i++ {
		tmpE := buf[i]
		buf[i] = rotateLeft(buf[i], byte((^uint32(prevEncrypted))%7))
		buf[i] ^= rotateLeft(byteKey[15-i%16], byte((int(prevEncrypted)+len(buf)-i)%7))

		// Handle negative results properly - convert to uint before modulo
		diff := int(buf[i]) - int(byteKey[i%16]>>2)
		buf[i] = byte((diff%256 + 256) % 256)

		prevEncrypted = tmpE
	}
}

// rotateLeft rotates a byte left by n bits
func rotateLeft(val, n byte) byte {
	return (val << n) | (val >> (8 - n))
}

// rotateRight rotates a byte right by n bits
func rotateRight(val, n byte) byte {
	return (val >> n) | (val << (8 - n))
}

// uint32SliceToByteSlice converts a uint32 slice to byte slice
func uint32SliceToByteSlice(u32s []uint32) []byte {
	bytes := make([]byte, len(u32s)*4)
	for i, u32 := range u32s {
		bytes[i*4] = byte(u32)
		bytes[i*4+1] = byte(u32 >> 8)
		bytes[i*4+2] = byte(u32 >> 16)
		bytes[i*4+3] = byte(u32 >> 24)
	}
	return bytes
}
