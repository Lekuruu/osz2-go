package osz2

import "errors"

// KeyType determines how encryption keys are generated for package operations.
type KeyType string

const (
	KeyTypeOsz2 KeyType = "osz2"
	KeyTypeOsf2 KeyType = "osf2"
)

func (key KeyType) Generate(metadata map[MetaType]string) ([]byte, error) {
	switch key {
	// Regular .osz2 files, mainly used for beatmap submission
	// Requires: Creator & BeatmapSetID metadata fields
	case KeyTypeOsz2:
		creator, okCreator := metadata[Creator]
		beatmapSetID, okBeatmapSetID := metadata[BeatmapSetID]
		if !okCreator || !okBeatmapSetID {
			return nil, errors.New("missing required metadata for osz2 key generation: Creator and BeatmapSetID")
		}
		seed := creator + "yhxyfjo5" + beatmapSetID
		return ComputeHashBytesRaw([]byte(seed)), nil
	// .osf2 files, used for beatmap packages inside osu!stream
	// Requires: Title & Artist metadata fields
	case KeyTypeOsf2:
		title, okTitle := metadata[Title]
		artist, okArtist := metadata[Artist]
		if !okTitle || !okArtist {
			return nil, errors.New("missing required metadata for osf2 key generation: Title and Artist")
		}
		seed := "\x08" + title + "4390gn8931i" + artist
		return ComputeHashBytesRaw([]byte(seed)), nil
	default:
		return nil, errors.New("unsupported key type")
	}
}
