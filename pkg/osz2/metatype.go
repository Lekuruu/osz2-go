package osz2

// MetaType represents the type of metadata in osz2 files
type MetaType int16

const (
	Title           MetaType = 0
	Artist          MetaType = 1
	Creator         MetaType = 2
	Version         MetaType = 3
	Source          MetaType = 4
	Tags            MetaType = 5
	VideoDataOffset MetaType = 6
	VideoDataLength MetaType = 7
	VideoHash       MetaType = 8
	BeatmapSetID    MetaType = 9
	Genre           MetaType = 10
	Language        MetaType = 11
	TitleUnicode    MetaType = 12
	ArtistUnicode   MetaType = 13
	Unknown         MetaType = 9999
	Difficulty      MetaType = 10000
	PreviewTime     MetaType = 10001
	ArtistFullName  MetaType = 10002
	ArtistTwitter   MetaType = 10003
	SourceUnicode   MetaType = 10004
	ArtistURL       MetaType = 10005
	Revision        MetaType = 10006
	PackID          MetaType = 10007
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
