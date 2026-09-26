package osz2

import (
	"bytes"
	"crypto/md5"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// TestPackagesOsz2 tests parsing of all .osz2 files in the tests directory
func TestPackagesOsz2(t *testing.T) {
	testFiles := []string{}

	// Walk the tests directory to find .osz2 files
	filepath.Walk("tests", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".osz2" {
			testFiles = append(testFiles, path)
		}
		return nil
	})

	for _, testFile := range testFiles {
		t.Run(testFile, func(t *testing.T) {
			testPackage(t, testFile, KeyTypeOsz2)
		})
	}
}

// TestPackagesOsf2 tests parsing of all .osf2 files in the tests directory
func TestPackagesOsf2(t *testing.T) {
	testFiles := []string{}

	// Walk the tests directory to find .osf2 files
	filepath.Walk("tests", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".osf2" {
			testFiles = append(testFiles, path)
		}
		return nil
	})

	for _, testFile := range testFiles {
		t.Run(testFile, func(t *testing.T) {
			testPackage(t, testFile, KeyTypeOsf2)
		})
	}
}

// testPackage tests parsing a single .osz2 file
func testPackage(t *testing.T, filename string, key KeyType) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("Test file does not exist: %s", filename)
	}

	t.Logf("Parsing package: %s", filename)
	pkg, err := OpenReader(filename, key)
	if err != nil {
		t.Fatalf("Failed to parse package %s: %v", filename, err)
	}
	defer pkg.Close()

	t.Logf("Metadata entries: %d", len(pkg.Metadata()))
	for metaType, value := range pkg.Metadata() {
		t.Logf("  %v: %s", metaType, value)
	}

	fs, err := pkg.ReadDir(".")
	if err != nil {
		t.Fatalf("Failed to read filesystem: %v", err)
	}

	for _, entry := range fs {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			t.Fatalf("Failed to read file info: %v", err)
		}
		t.Logf("  -> %s (%d bytes)", entry.Name(), info.Size())

		// Try to read the file
		file, err := pkg.Open(entry.Name())
		if err != nil {
			t.Fatalf("Failed to open file handle: %v", err)
		}

		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("Failed to read file contents: %v", err)
		}
		if len(data) != int(info.Size()) {
			t.Errorf("Invalid file size: got %d, want %d", len(data), info.Size())
		}
	}
}

func TestPackageRoundTrip(t *testing.T) {
	testFiles, err := filepath.Glob("tests/*.osz2")
	if err != nil {
		t.Fatal(err)
	}
	if len(testFiles) == 0 {
		t.Fatal("no osz2 test packages found")
	}

	for _, testFile := range testFiles {
		t.Run(filepath.Base(testFile), func(t *testing.T) {
			originalData, err := os.ReadFile(testFile)
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
	testFile := "tests/nekodex - welcome to christmas.osz2"

	stat, err := os.Stat(testFile)
	if os.IsNotExist(err) {
		b.Skip("Test file does not exist")
	}
	if err != nil {
		b.Errorf("Failed to open test file: %v", err)
	}

	for b.Loop() {
		file, err := os.Open(testFile)
		if err != nil {
			b.Fatalf("Failed to open file: %v", err)
		}

		_, err = NewReader(file, stat.Size(), KeyTypeOsz2)
		if err != nil {
			b.Fatalf("Failed to parse package: %v", err)
		}

		file.Close()
	}
}

func packageChecksums(t *testing.T, reader *Reader) map[string][md5.Size]byte {
	t.Helper()

	checksums := make(map[string][md5.Size]byte)
	err := fs.WalkDir(reader, ".", func(name string, dirEntry fs.DirEntry, err error) error {
		if err != nil || dirEntry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(reader, name)
		if err != nil {
			return err
		}
		checksums[name] = md5.Sum(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return checksums
}
