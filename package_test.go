package osz2

import (
	"io"
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
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("Test file does not exist: %s", filename)
	}

	// Parse the package
	t.Logf("Parsing package: %s", filename)
	pkg, err := OpenReader(filename, key)
	if err != nil {
		t.Fatalf("Failed to parse package %s: %v", filename, err)
	}
	defer pkg.Close()

	// Log metadata
	t.Logf("Metadata entries: %d", len(pkg.Metadata()))
	for metaType, value := range pkg.Metadata() {
		t.Logf("  %v: %s", metaType, value)
	}

	// Check each file
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

// BenchmarkParsePackage benchmarks package parsing
func BenchmarkParsePackage(b *testing.B) {
	testFile := "tests/nekodex - welcome to christmas.osz2"

	// Check if file exists
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

// TODO: Test read -> write -> read rountrip once we can write
