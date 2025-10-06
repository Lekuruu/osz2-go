package osz2

import (
	"crypto/md5"
)

// ComputeHash computes MD5 hash of a string
func ComputeHash(str string) string {
	return ComputeHashBytes([]byte(str))
}

// ComputeHashBytes computes MD5 hash of byte array and returns hex string
func ComputeHashBytes(data []byte) string {
	hash := md5.Sum(data)
	result := ""
	for _, b := range hash[:] {
		result += byteToHex(b)
	}
	return result
}

// ComputeHashBytesRaw computes MD5 hash and returns raw bytes
func ComputeHashBytesRaw(data []byte) []byte {
	hash := md5.Sum(data)
	return hash[:]
}

// byteToHex is a simple implementation for formatting hex
func byteToHex(b byte) string {
	hex := "0123456789abcdef"
	return string([]byte{hex[b>>4], hex[b&0xf]})
}
