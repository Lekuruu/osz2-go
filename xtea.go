package osz2

import (
	"encoding/binary"
)

// XTEA implements the Extended Tiny Encryption Algorithm
type XTEA struct {
	key           []uint32
	simpleCryptor *SimpleCryptor
}

// NewXTEA creates a new XTEA instance
func NewXTEA(key []uint32) *XTEA {
	return &XTEA{
		key:           key,
		simpleCryptor: NewSimpleCryptor(key),
	}
}

// Decrypt decrypts data using XTEA
func (x *XTEA) Decrypt(buffer []byte, start, count int) {
	x.encryptDecrypt(buffer, start, count, false)
}

// encryptDecrypt performs encryption or decryption
func (x *XTEA) encryptDecrypt(buffer []byte, bufStart, count int, encrypt bool) {
	fullWordCount := count / 8
	leftOver := count % 8

	// Process full 8-byte words
	for i := 0; i < fullWordCount; i++ {
		offset := bufStart + i*8
		v0 := binary.LittleEndian.Uint32(buffer[offset:])
		v1 := binary.LittleEndian.Uint32(buffer[offset+4:])

		if encrypt {
			v0, v1 = x.encryptWord(v0, v1)
		} else {
			v0, v1 = x.decryptWord(v0, v1)
		}

		binary.LittleEndian.PutUint32(buffer[offset:], v0)
		binary.LittleEndian.PutUint32(buffer[offset+4:], v1)
	}

	// Handle leftover bytes
	if leftOver > 0 {
		leftoverStart := bufStart + fullWordCount*8
		leftoverBuf := buffer[leftoverStart : leftoverStart+leftOver]
		if encrypt {
			x.simpleCryptor.EncryptBytes(leftoverBuf)
		} else {
			x.simpleCryptor.DecryptBytes(leftoverBuf)
		}
	}
}

// encryptWord encrypts a single 64-bit word (two 32-bit values)
func (x *XTEA) encryptWord(v0, v1 uint32) (uint32, uint32) {
	var sum uint32 = 0
	for i := uint32(0); i < TEARounds; i++ {
		v0 += (((v1 << 4) ^ (v1 >> 5)) + v1) ^ (sum + x.key[sum&3])
		sum += TEADelta
		v1 += (((v0 << 4) ^ (v0 >> 5)) + v0) ^ (sum + x.key[(sum>>11)&3])
	}
	return v0, v1
}

// decryptWord decrypts a single 64-bit word (two 32-bit values)
func (x *XTEA) decryptWord(v0, v1 uint32) (uint32, uint32) {
	// Calculate sum with proper overflow handling
	sum := uint32(0)
	for i := uint32(0); i < TEARounds; i++ {
		sum += TEADelta
	}

	for i := uint32(0); i < TEARounds; i++ {
		v1 -= (((v0 << 4) ^ (v0 >> 5)) + v0) ^ (sum + x.key[(sum>>11)&3])
		sum -= TEADelta
		v0 -= (((v1 << 4) ^ (v1 >> 5)) + v1) ^ (sum + x.key[sum&3])
	}
	return v0, v1
}
