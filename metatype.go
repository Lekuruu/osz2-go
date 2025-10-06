package osz2

// MetaType represents the type of metadata in osz2 files
type MetaType int16

const (
	Title MetaType = iota
	Artist
	Creator
	Version
	Source
	Tags
	VideoDataOffset
	VideoDataLength
	VideoHash
	BeatmapSetID
	Genre
	Language
	TitleUnicode
	ArtistUnicode
	Difficulty
	PreviewTime
	ArtistFullName
	ArtistTwitter
	SourceUnicode
	ArtistURL
	Revision
	PackID
	Unknown MetaType = 9999
)

// String returns the string representation of MetaType
func (m MetaType) String() string {
	switch m {
	case Title:
		return "Title"
	case Artist:
		return "Artist"
	case Creator:
		return "Creator"
	case Version:
		return "Version"
	case Source:
		return "Source"
	case Tags:
		return "Tags"
	case VideoDataOffset:
		return "VideoDataOffset"
	case VideoDataLength:
		return "VideoDataLength"
	case VideoHash:
		return "VideoHash"
	case BeatmapSetID:
		return "BeatmapSetID"
	case Genre:
		return "Genre"
	case Language:
		return "Language"
	case TitleUnicode:
		return "TitleUnicode"
	case ArtistUnicode:
		return "ArtistUnicode"
	case Difficulty:
		return "Difficulty"
	case PreviewTime:
		return "PreviewTime"
	case ArtistFullName:
		return "ArtistFullName"
	case ArtistTwitter:
		return "ArtistTwitter"
	case SourceUnicode:
		return "SourceUnicode"
	case ArtistURL:
		return "ArtistURL"
	case Revision:
		return "Revision"
	case PackID:
		return "PackID"
	default:
		return "Unknown"
	}
}
