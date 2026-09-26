package osz2

import (
	"crypto/md5"
	"io"
	"io/fs"

	"github.com/Lekuruu/osz2-go/internal/crypto"
)

func (f *File) readAt(p []byte, offset int64) (int, error) {
	if offset < 0 {
		return 0, f.pathError("read", fs.ErrInvalid)
	}
	return decryptFrameAt(
		f.reader.source,
		f.entry.dataOffset,
		f.entry.size,
		f.reader.key,
		p,
		offset,
	)
}

func decryptFrameAt(
	source io.ReaderAt,
	start, length int64,
	key [md5.Size]byte,
	p []byte,
	offset int64,
) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if offset < 0 {
		return 0, fs.ErrInvalid
	}
	if offset >= length {
		return 0, io.EOF
	}

	// XXTEA encrypts each block independently, so reads must
	// decrypt the entire block containing the requested bytes.
	// The final block may be shorter than maxBytes, so its exact
	// length must be preserved.

	want := min(int64(len(p)), length-offset)
	cryptor := crypto.NewXXTEA(bytesToUint32s(key[:]))

	var buf [crypto.MaxBytes]byte
	written := 0

	for int64(written) < want {
		pos := offset + int64(written)

		blockStart := pos / crypto.MaxBytes * crypto.MaxBytes
		blockLength := min(int64(crypto.MaxBytes), length-blockStart)
		block := buf[:blockLength]

		if _, err := source.ReadAt(block, start+blockStart); err != nil {
			return written, err
		}

		cryptor.Decrypt(block, 0, len(block))

		inside := pos - blockStart
		count := min(blockLength-inside, want-int64(written))

		copy(p[written:], block[inside:inside+count])
		written += int(count)
	}

	if written < len(p) {
		return written, io.EOF
	}
	return written, nil
}
