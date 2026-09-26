package osz2

import (
	"bytes"
	"crypto/md5"
	"hash"
)

// computeOszHash calculates the modified md5 hash used by the osz2 format.
func computeOszHash(data []byte, position int, mask byte) [md5.Size]byte {
	input := bytes.Clone(data)
	if position < len(input) {
		input[position] ^= mask
	}
	return finalizeOszHash(md5.Sum(input))
}

// finalizeOszHash applies the osz2 format's transformation to an md5 hash.
func finalizeOszHash(result [md5.Size]byte) [md5.Size]byte {
	for i := range md5.Size / 2 {
		result[i], result[i+md5.Size/2] = result[i+md5.Size/2], result[i]
	}
	result[5] ^= 0x2d
	return result
}

// bodyHasher calculates the package body hash while data is being written.
type bodyHasher struct {
	hash hash.Hash

	physicalOffset int64
	logicalOffset  int64

	excludeStart int64
	excludeEnd   int64

	maskPosition int64
}

func newBodyHasher(total, excludeStart, excludeLength int64) *bodyHasher {
	length := total
	if excludeStart >= 0 {
		length -= excludeLength
	}
	return &bodyHasher{
		hash:         md5.New(),
		excludeStart: excludeStart,
		excludeEnd:   excludeStart + excludeLength,
		maskPosition: length / 2,
	}
}

func (h *bodyHasher) Write(p []byte) (int, error) {
	written := len(p)

	for len(p) > 0 {
		// Skip bytes that are excluded from the body hash
		if h.inExcludedRange() {
			n := min(int64(len(p)), h.excludeEnd-h.physicalOffset)
			h.physicalOffset += n
			p = p[n:]
			continue
		}
		n := int64(len(p))

		// Stop before the excluded range so it can be skipped separately
		if h.excludeStart >= 0 && h.physicalOffset < h.excludeStart {
			n = min(n, h.excludeStart-h.physicalOffset)
		}

		h.writeHashed(p[:n])
		h.physicalOffset += n
		p = p[n:]
	}

	return written, nil
}

func (h *bodyHasher) inExcludedRange() bool {
	return h.excludeStart >= 0 &&
		h.physicalOffset >= h.excludeStart &&
		h.physicalOffset < h.excludeEnd
}

func (h *bodyHasher) writeHashed(p []byte) {
	start := h.logicalOffset
	end := start + int64(len(p))

	if h.maskPosition >= start && h.maskPosition < end {
		index := h.maskPosition - start
		h.hash.Write(p[:index])
		h.hash.Write([]byte{p[index] ^ 0x9f})
		h.hash.Write(p[index+1:])
	} else {
		h.hash.Write(p)
	}

	h.logicalOffset = end
}

func (h *bodyHasher) sum() [md5.Size]byte {
	var result [md5.Size]byte
	copy(result[:], h.hash.Sum(nil))
	return finalizeOszHash(result)
}
