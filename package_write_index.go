package osz2

import (
	"cmp"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	videoHashWindowSize = 1024
	videoHashAlignment  = 16
)

func (w *Writer) indexSourceEntry(
	source fs.FS,
	name string,
	dirEntry fs.DirEntry,
	fallbackTime time.Time,
) (*writeEntry, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, fmt.Errorf("osz2: invalid source path %q", name)
	}

	info, err := dirEntry.Info()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("osz2: source entry %q is not a regular file", name)
	}
	if info.Size() < 0 || info.Size() > 1<<31-5 {
		return nil, fmt.Errorf(
			"osz2: source entry %q has unsupported size %d",
			name, info.Size(),
		)
	}

	modifiedAt := info.ModTime()
	if modifiedAt.IsZero() {
		modifiedAt = fallbackTime
	}
	createdAt := modifiedAt

	createdAt = createdAt.UTC()
	modifiedAt = modifiedAt.UTC()

	if err := validateEntryTimes(name, createdAt, modifiedAt); err != nil {
		return nil, err
	}

	entry := &writeEntry{
		path:       name,
		size:       info.Size(),
		createdAt:  createdAt,
		modifiedAt: modifiedAt,
		beatmapID:  -1,
	}

	entry.hash, entry.videoHash, err = inspectSourceEntry(source, entry)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

func (w *Writer) finalizeIndex() error {
	// Match osu!stream's file order, i.e. non-video files first, then videos.
	// https://github.com/ppy/osu-stream/blob/master/osu!stream/Helpers/osu!common/MapPackage.cs#
	// The format records offset, length, and hash metadata for only one video.
	slices.SortFunc(w.entries, func(a, b *writeEntry) int {
		if a.isVideo() != b.isVideo() {
			if a.isVideo() {
				return 1
			}
			return -1
		}
		return cmp.Compare(a.path, b.path)
	})

	var offset int64
	for _, entry := range w.entries {
		entry.offset = int32(offset)
		offset += 4 + entry.size

		if offset >= 1<<31 {
			return errors.New("osz2: package file data exceeds int32 offset range")
		}
	}

	for _, entry := range w.entries {
		if !entry.isVideo() {
			continue
		}

		w.metadata[VideoDataOffset] = strconv.FormatInt(int64(entry.offset), 10)
		w.metadata[VideoDataLength] = strconv.FormatInt(entry.size, 10)
		w.metadata[VideoHash] = fmt.Sprintf("%X", entry.videoHash[:])
		break
	}

	return nil
}

func (w *Writer) validateBeatmapIDs() error {
	beatmapIDs := make(map[int32]string)

	for _, entry := range w.entries {
		if !entry.isBeatmapFile() || entry.beatmapID == -1 {
			continue
		}
		if other, exists := beatmapIDs[entry.beatmapID]; exists {
			return fmt.Errorf(
				"osz2: beatmap ID %d is assigned to both %q and %q",
				entry.beatmapID, other, entry.path,
			)
		}
		beatmapIDs[entry.beatmapID] = entry.path
	}

	return nil
}

func validateEntryTimes(name string, createdAt, modifiedAt time.Time) error {
	if !createdAt.IsZero() {
		if _, err := datetimeToDotNetBinary(createdAt); err != nil {
			return fmt.Errorf("osz2: creation time for %q: %w", name, err)
		}
	}
	if !modifiedAt.IsZero() {
		if _, err := datetimeToDotNetBinary(modifiedAt); err != nil {
			return fmt.Errorf("osz2: modification time for %q: %w", name, err)
		}
	}
	return nil
}

func inspectSourceEntry(source fs.FS, entry *writeEntry) ([md5.Size]byte, [md5.Size]byte, error) {
	file, err := source.Open(entry.path)
	if err != nil {
		return [md5.Size]byte{}, [md5.Size]byte{}, err
	}
	defer file.Close()

	hasher := md5.New()
	buffer := make([]byte, 64*1024)

	var videoWindow []byte
	var videoStart int64

	if entry.isVideo() {
		if entry.size < videoHashWindowSize {
			return [md5.Size]byte{}, [md5.Size]byte{}, fmt.Errorf(
				"osz2: video %q must be at least %d bytes",
				entry.path,
				videoHashWindowSize,
			)
		}

		// osu!stream hashes a 1024-byte window starting on a 16-byte boundary near the middle of the video:
		// https://github.com/ppy/osu-stream/blob/master/osu!stream/Helpers/osu!common/MapPackage.cs#L966-L977

		half := entry.size / 2
		videoStart = half -
			half%videoHashAlignment -
			videoHashWindowSize/2 +
			videoHashAlignment

		if videoStart < 0 || videoStart+videoHashWindowSize > entry.size {
			return [md5.Size]byte{}, [md5.Size]byte{}, fmt.Errorf(
				"osz2: video %q is too short for its %d-byte hash window",
				entry.path, videoHashWindowSize,
			)
		}

		videoWindow = make([]byte, videoHashWindowSize)
	}

	var bytesRead int64

	for {
		n, err := file.Read(buffer)

		if n > 0 {
			data := buffer[:n]
			hasher.Write(data)

			if videoWindow != nil {
				copyOverlap(videoWindow, videoStart, data, bytesRead)
			}
			bytesRead += int64(n)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return [md5.Size]byte{}, [md5.Size]byte{}, err
		}
		if n == 0 {
			return [md5.Size]byte{}, [md5.Size]byte{}, io.ErrNoProgress
		}
	}

	if bytesRead != entry.size {
		return [md5.Size]byte{}, [md5.Size]byte{}, fmt.Errorf(
			"osz2: size of %q changed from %d to %d",
			entry.path,
			entry.size,
			bytesRead,
		)
	}

	var contentHash [md5.Size]byte
	copy(contentHash[:], hasher.Sum(nil))

	var videoHash [md5.Size]byte
	if videoWindow != nil {
		videoHash = md5.Sum(videoWindow)
	}

	return contentHash, videoHash, nil
}

func copyOverlap(dst []byte, dstStart int64, src []byte, srcStart int64) {
	start := max(dstStart, srcStart)
	end := min(
		dstStart+int64(len(dst)),
		srcStart+int64(len(src)),
	)

	if start >= end {
		return
	}

	copy(
		dst[start-dstStart:end-dstStart],
		src[start-srcStart:end-srcStart],
	)
}

func (e *writeEntry) extension() string {
	ext := path.Ext(e.path)
	if ext == "" {
		return ""
	}
	return strings.ToLower(ext[1:])
}

func (e *writeEntry) isBeatmapFile() bool {
	switch e.extension() {
	case "osu", "osc":
		return true
	default:
		return false
	}
}

func (e *writeEntry) isVideo() bool {
	_, ok := videoFileExtensions[e.extension()]
	return ok
}
