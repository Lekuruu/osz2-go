package osz2

import (
	"crypto/md5"
	"fmt"
	"io"

	"github.com/Lekuruu/osz2-go/internal/crypto"
)

// Verify checks this entry against the hash stored in the package.
func (f *File) Verify() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.checkReadable("verify"); err != nil {
		return err
	}

	hash, err := f.hash()
	if err != nil {
		return err
	}
	if hash == f.entry.hash {
		return nil
	}

	// Older packages created by the og C# implementation calculated
	// the hash using a different decryption framing:
	// it included the 4 bytes before the file data in the encrypted
	// stream, then skipped those bytes after decryption.
	// We want to account for both methods.
	hash, err = hashLegacy(
		f.reader.source,
		f.entry.dataOffset-4,
		f.entry.size+4,
		f.reader.key,
	)
	if err != nil {
		return f.pathError("verify", err)
	}
	if hash != f.entry.hash {
		return f.pathError("verify", fmt.Errorf("content hash mismatch"))
	}
	return nil
}

func (f *File) hash() ([md5.Size]byte, error) {
	hasher := md5.New()
	buffer := make([]byte, 64*1024)

	var offset int64
	for offset < f.entry.size {
		n, err := f.readAt(buffer, offset)
		if n > 0 {
			_, _ = hasher.Write(buffer[:n])
			offset += int64(n)
		}
		if err != nil && err != io.EOF {
			return [md5.Size]byte{}, err
		}
		if n == 0 {
			return [md5.Size]byte{}, f.pathError("verify", io.ErrUnexpectedEOF)
		}
	}

	var result [md5.Size]byte
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

func hashLegacy(source io.ReaderAt, start, length int64, key [md5.Size]byte) ([md5.Size]byte, error) {
	stream := io.NewSectionReader(source, start, length)
	hasher := md5.New()
	cryptor := crypto.NewXXTEA(bytesToUint32s(key[:]))

	// The legacy implementation decrypts the 4-byte prefix together with the file data.
	// We want to preserve that framing for decryption, but exclude the prefix itself from the hash.
	skip := int64(4)

	var buffer [crypto.MaxBytes]byte
	for remaining := length; remaining > 0; {
		blockLength := min(int64(crypto.MaxBytes), remaining)
		block := buffer[:blockLength]

		if _, err := io.ReadFull(stream, block); err != nil {
			return [md5.Size]byte{}, err
		}

		cryptor.Decrypt(block, 0, len(block))

		if skip >= blockLength {
			skip -= blockLength
		} else {
			_, _ = hasher.Write(block[skip:])
			skip = 0
		}

		remaining -= blockLength
	}

	var result [md5.Size]byte
	copy(result[:], hasher.Sum(nil))
	return result, nil
}
