package osz2

import (
	"crypto/md5"
	"errors"
	"fmt"
)

const (
	metadataHashPositionMultiplier = 3
	fileInfoHashPositionMultiplier = 4

	metadataHashMask     byte = 0xa7
	fileInfoHashMask     byte = 0xd1
	bodyHashMask         byte = 0x9f
	hashFinalizationMask byte = 0x2d

	fileInfoLengthShift          = 17
	maximumEncodedFileInfoLength = 1<<31 - 1
)

func computeMetadataHash(data []byte, entryCount int) [md5.Size]byte {
	return computeOszHash(
		data,
		entryCount*metadataHashPositionMultiplier,
		metadataHashMask,
	)
}

func computeFileInfoHash(data []byte, entryCount int) [md5.Size]byte {
	return computeOszHash(
		data,
		entryCount*fileInfoHashPositionMultiplier,
		fileInfoHashMask,
	)
}

func encodeFileInfoLength(length int, hash [md5.Size]byte) (int32, error) {
	adjustment := fileInfoLengthAdjustment(hash)
	if int64(length) > maximumEncodedFileInfoLength-adjustment {
		return 0, errors.New("osz2: encoded file-info length overflows int32")
	}
	encoded := int64(length) + adjustment
	return int32(encoded), nil
}

func decodeFileInfoLength(encoded int32, hash [md5.Size]byte) (int64, error) {
	length := int64(encoded) - fileInfoLengthAdjustment(hash)
	if length < 0 {
		return 0, fmt.Errorf("osz2: invalid file-info length %d", length)
	}
	return length, nil
}

func fileInfoLengthAdjustment(hash [md5.Size]byte) int64 {
	var adjustment int64
	for i := 0; i < md5.Size; i += 2 {
		adjustment += int64(hash[i]) | int64(hash[i+1])<<fileInfoLengthShift
	}
	return adjustment
}
