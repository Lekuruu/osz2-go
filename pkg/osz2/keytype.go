package osz2

import (
	"crypto/md5"
	"errors"
)

// KeyType determines how encryption keys are generated for package operations.
type KeyType string

const (
	// KeyTypeOsz2 is used by regular bss packages.
	KeyTypeOsz2 KeyType = "osz2"
	// KeyTypeOsf2 is used by osu!stream packages.
	KeyTypeOsf2 KeyType = "osf2"
)

// String returns the conventional lowercase format name.
func (keyType KeyType) String() string {
	switch keyType {
	case KeyTypeOsz2:
		return "osz2"
	case KeyTypeOsf2:
		return "osf2"
	default:
		return "unknown"
	}
}

// Generate derives the 16-byte package encryption key from its metadata.
func (keyType KeyType) Generate(metadata Metadata) ([md5.Size]byte, error) {
	switch keyType {
	// Regular .osz2 files, mainly used for beatmap submission
	// Requires: Creator & BeatmapSetID metadata fields
	case KeyTypeOsz2:
		creator, okCreator := metadata[Creator]
		beatmapSetID, okBeatmapSetID := metadata[BeatmapSetID]
		if !okCreator || !okBeatmapSetID {
			return [md5.Size]byte{}, errors.New("missing metadata for osz2 keygen: Creator and BeatmapSetID")
		}
		seed := creator + "yhxyfjo5" + beatmapSetID
		return md5.Sum(encodeToASCII(seed)), nil
	// .osf2 files, used for beatmap packages inside osu!stream
	// Requires: Title & Artist metadata fields
	case KeyTypeOsf2:
		title, okTitle := metadata[Title]
		artist, okArtist := metadata[Artist]
		if !okTitle || !okArtist {
			return [md5.Size]byte{}, errors.New("missing metadata for osf2 keygen: Title and Artist")
		}
		seed := "\x08" + title + "4390gn8931i" + artist
		return md5.Sum(encodeToASCII(seed)), nil
	default:
		return [md5.Size]byte{}, errors.New("unsupported key type")
	}
}

func encodeToASCII(value string) []byte {
	result := make([]byte, 0, len(value))
	for _, r := range value {
		if r > 0x7f {
			// C# is using Encoding.ASCII which replaces non-ascii characters with '?'
			result = append(result, '?')
			continue
		}
		result = append(result, byte(r))
	}
	return result
}
