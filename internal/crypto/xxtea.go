package crypto

import (
	"encoding/binary"
)

type XXTEA struct {
	key           []uint32
	simpleCryptor *SimpleCryptor
	n             uint32
}

const (
	MaxWords = 16
	MaxBytes = MaxWords * 4
)

func NewXXTEA(key []uint32) *XXTEA {
	return &XXTEA{
		key:           key,
		simpleCryptor: NewSimpleCryptor(key),
	}
}

// Decrypt decrypts data using XXTEA
func (xx *XXTEA) Decrypt(buffer []byte, start, count int) {
	xx.encryptDecrypt(buffer, start, count, false)
}

// Encrypt encrypts data using XXTEA
func (xx *XXTEA) Encrypt(buffer []byte, start, count int) {
	xx.encryptDecrypt(buffer, start, count, true)
}

// encryptDecrypt performs XXTEA encryption or decryption
func (xx *XXTEA) encryptDecrypt(buffer []byte, bufStart, count int, encrypt bool) {
	fullWordCount := count / MaxBytes
	leftOver := count % MaxBytes

	// Process full 64-byte chunks. Each chunk contains 16 uint32 values.
	for i := range fullWordCount {
		offset := bufStart + i*MaxBytes
		if encrypt {
			xx.encryptFixedWordArray(buffer[offset : offset+MaxBytes])
		} else {
			xx.decryptFixedWordArray(buffer[offset : offset+MaxBytes])
		}
	}

	if leftOver == 0 {
		return
	}

	// Handle leftover bytes
	leftoverStart := bufStart + fullWordCount*MaxBytes
	xx.n = uint32(leftOver / 4)

	if xx.n > 1 {
		if encrypt {
			xx.encryptWords(buffer[leftoverStart : leftoverStart+int(xx.n)*4])
		} else {
			xx.decryptWords(buffer[leftoverStart : leftoverStart+int(xx.n)*4])
		}

		leftOver -= int(xx.n) * 4
		if leftOver == 0 {
			return
		}
		leftoverStart += int(xx.n) * 4
	}

	// Handle remaining bytes with simple cryptor
	// When n <= 1, this processes ALL leftover bytes starting from leftoverStart
	remainingBuf := buffer[leftoverStart : leftoverStart+leftOver]
	if encrypt {
		xx.simpleCryptor.encryptBytes(remainingBuf)
	} else {
		xx.simpleCryptor.decryptBytes(remainingBuf)
	}
}

// encryptWords encrypts a block of words using XXTEA
func (xx *XXTEA) encryptWords(data []byte) {
	if len(data) < int(xx.n)*4 {
		return
	}

	// Convert bytes to uint32 array
	var storage [MaxWords]uint32
	v := storage[:xx.n]
	for i := range xx.n {
		v[i] = binary.LittleEndian.Uint32(data[i*4:])
	}

	var y, z, sum uint32
	var p, e uint32
	rounds := 6 + 52/xx.n
	sum = 0
	z = v[xx.n-1]

	for rounds > 0 {
		sum += TEADelta
		e = (sum >> 2) & 3
		for p = 0; p < xx.n-1; p++ {
			y = v[p+1]
			v[p] += (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
			z = v[p]
		}
		y = v[0]
		v[xx.n-1] += (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
		z = v[xx.n-1]
		rounds--
	}

	// Convert back to bytes
	for i := range xx.n {
		binary.LittleEndian.PutUint32(data[i*4:], v[i])
	}
}

// decryptWords decrypts a block of words using XXTEA
func (xx *XXTEA) decryptWords(data []byte) {
	if len(data) < int(xx.n)*4 {
		return
	}

	// Convert bytes to uint32 array
	var storage [MaxWords]uint32
	v := storage[:xx.n]
	for i := range xx.n {
		v[i] = binary.LittleEndian.Uint32(data[i*4:])
	}

	var y, z, sum uint32
	var p, e uint32
	rounds := 6 + 52/xx.n

	// Calculate initial sum
	sum = rounds * TEADelta
	y = v[0]

	for {
		e = (sum >> 2) & 3
		for p = xx.n - 1; p > 0; p-- {
			z = v[p-1]
			v[p] -= (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
			y = v[p]
		}
		z = v[xx.n-1]
		v[0] -= (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
		y = v[0]

		sum -= TEADelta
		if sum == 0 {
			break
		}
	}

	// Convert back to bytes
	for i := range xx.n {
		binary.LittleEndian.PutUint32(data[i*4:], v[i])
	}
}

// encryptFixedWordArray encrypts one fixed 16-word block using XXTEA.
func (xx *XXTEA) encryptFixedWordArray(data []byte) {
	if len(data) != MaxBytes {
		return
	}

	// Convert bytes to uint32 array
	var v [MaxWords]uint32
	for i := range MaxWords {
		v[i] = binary.LittleEndian.Uint32(data[i*4:])
	}

	var y, z, sum uint32
	var p, e uint32
	rounds := 6 + 52/MaxWords
	sum = 0
	z = v[MaxWords-1]

	for rounds > 0 {
		sum += TEADelta
		e = (sum >> 2) & 3
		for p = 0; p < MaxWords-1; p++ {
			y = v[p+1]
			v[p] += (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
			z = v[p]
		}
		y = v[0]
		v[MaxWords-1] += (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
		z = v[MaxWords-1]
		rounds--
	}

	// Convert back to bytes
	for i := range MaxWords {
		binary.LittleEndian.PutUint32(data[i*4:], v[i])
	}
}

// decryptFixedWordArray decrypts one fixed 16-word block using XXTEA.
func (xx *XXTEA) decryptFixedWordArray(data []byte) {
	if len(data) != MaxBytes {
		return
	}

	// Convert bytes to uint32 array
	var v [MaxWords]uint32
	for i := range MaxWords {
		v[i] = binary.LittleEndian.Uint32(data[i*4:])
	}

	var y, z, sum uint32
	var p, e uint32
	rounds := 6 + 52/MaxWords

	// Calculate initial sum
	sum = uint32(rounds) * TEADelta
	y = v[0]

	for {
		e = (sum >> 2) & 3
		for p = MaxWords - 1; p > 0; p-- {
			z = v[p-1]
			v[p] -= (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
			y = v[p]
		}
		z = v[MaxWords-1]
		v[0] -= (((z >> 5) ^ (y << 2)) + ((y >> 3) ^ (z << 4))) ^ ((sum ^ y) + (xx.key[(p&3)^e] ^ z))
		y = v[0]

		sum -= TEADelta
		if sum == 0 {
			break
		}
	}

	// Convert back to bytes
	for i := range MaxWords {
		binary.LittleEndian.PutUint32(data[i*4:], v[i])
	}
}
