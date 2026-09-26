package osz2

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"testing"
)

var testFixtures = os.DirFS("../../tests")

// TestPackagesOsz2 tests parsing of all .osz2 files in the tests directory
func TestPackagesOsz2(t *testing.T) {
	for _, testFile := range testPackageNames(t, "*.osz2") {
		t.Run(testFile, func(t *testing.T) {
			testPackage(t, testFile, KeyTypeOsz2)
		})
	}
}

// TestPackagesOsf2 tests parsing of all .osf2 files in the tests directory
func TestPackagesOsf2(t *testing.T) {
	for _, testFile := range testPackageNames(t, "*.osf2") {
		t.Run(testFile, func(t *testing.T) {
			testPackage(t, testFile, KeyTypeOsf2)
		})
	}
}

func testPackageNames(t testing.TB, pattern string) []string {
	t.Helper()

	names, err := fs.Glob(testFixtures, pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatalf("no test packages found matching %q", pattern)
	}
	return names
}

// testPackage tests parsing a single .osz2 / .osf2 file
func testPackage(t *testing.T, filename string, keyType KeyType) {
	data, err := fs.ReadFile(testFixtures, filename)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Parsing package: %s", filename)
	pkg, err := NewReader(bytes.NewReader(data), int64(len(data)), keyType)
	if err != nil {
		t.Fatalf("Failed to parse package %s: %v", filename, err)
	}

	t.Logf("Metadata entries: %d", len(pkg.Metadata()))
	for metaType, value := range pkg.Metadata() {
		t.Logf("  %v: %s", metaType, value)
	}

	err = fs.WalkDir(pkg, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		t.Logf("  -> %s (%d bytes)", name, info.Size())

		// Try to read the file
		file, err := pkg.Open(name)
		if err != nil {
			return err
		}
		bytesRead, readErr := io.Copy(io.Discard, file)
		if err := errors.Join(readErr, file.Close()); err != nil {
			return err
		}
		if bytesRead != info.Size() {
			return fmt.Errorf(
				"decrypted size for %q is %d, entry says %d",
				name, bytesRead, info.Size(),
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to read filesystem: %v", err)
	}
}

func TestPackageRoundTrip(t *testing.T) {
	for _, testFile := range testPackageNames(t, "*.osz2") {
		t.Run(testFile, func(t *testing.T) {
			originalData, err := fs.ReadFile(testFixtures, testFile)
			if err != nil {
				t.Fatal(err)
			}

			original, err := NewReader(
				bytes.NewReader(originalData),
				int64(len(originalData)),
				KeyTypeOsz2,
			)
			if err != nil {
				t.Fatal(err)
			}

			var rewrittenData bytes.Buffer
			writer, err := NewWriter(&rewrittenData)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.SetKey(original.Info().KeyType); err != nil {
				t.Fatal(err)
			}
			if err := writer.SetVersion(original.Info().Version); err != nil {
				t.Fatal(err)
			}
			for metadataType, value := range original.Metadata() {
				if err := writer.SetMetadata(metadataType, value); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.SetFS(original); err != nil {
				t.Fatal(err)
			}

			err = fs.WalkDir(original, ".", func(name string, dirEntry fs.DirEntry, err error) error {
				if err != nil || dirEntry.IsDir() {
					return err
				}
				entry, err := original.Entry(name)
				if err != nil {
					return err
				}
				if err := writer.AssignTimes(name, entry.CreatedAt(), entry.ModTime()); err != nil {
					return err
				}
				if beatmapID, ok := entry.BeatmapID(); ok {
					return writer.AssignBeatmapID(name, beatmapID)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			rewritten, err := NewReader(
				bytes.NewReader(rewrittenData.Bytes()),
				int64(rewrittenData.Len()),
				KeyTypeOsz2,
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := rewritten.Verify(); err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(original.Metadata(), rewritten.Metadata()) {
				t.Fatalf("metadata mismatch, original: %v, rewritten: %v", original.Metadata(), rewritten.Metadata())
			}

			originalChecksums := packageChecksums(t, original)
			rewrittenChecksums := packageChecksums(t, rewritten)
			if !maps.Equal(originalChecksums, rewrittenChecksums) {
				t.Fatalf("file checksum mismatch, original: %v,rewritten: %v", originalChecksums, rewrittenChecksums)
			}
		})
	}
}

func BenchmarkParsePackage(b *testing.B) {
	data, err := fs.ReadFile(testFixtures, "nekodex - welcome to christmas.osz2")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := NewReader(bytes.NewReader(data), int64(len(data)), KeyTypeOsz2)
		if err != nil {
			b.Fatalf("Failed to parse package: %v", err)
		}
	}
}

func packageChecksums(t *testing.T, reader *Reader) map[string][md5.Size]byte {
	t.Helper()

	checksums := make(map[string][md5.Size]byte)
	err := fs.WalkDir(reader, ".", func(name string, dirEntry fs.DirEntry, err error) error {
		if err != nil || dirEntry.IsDir() {
			return err
		}
		file, err := reader.Open(name)
		if err != nil {
			return err
		}

		hasher := md5.New()
		_, hashErr := io.Copy(hasher, file)
		if err := errors.Join(hashErr, file.Close()); err != nil {
			return err
		}

		var checksum [md5.Size]byte
		copy(checksum[:], hasher.Sum(nil))
		checksums[name] = checksum
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return checksums
}
