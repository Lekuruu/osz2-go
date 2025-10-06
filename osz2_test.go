package osz2

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPackages tests parsing of all .osz2 files in the tests directory
func TestPackages(t *testing.T) {
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
			testPackage(t, testFile)
		})
	}
}

// testPackage tests parsing a single .osz2 file
func testPackage(t *testing.T, filename string) {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("Test file does not exist: %s", filename)
	}

	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("Failed to open file %s: %v", filename, err)
	}
	defer file.Close()

	// Parse the package
	t.Logf("Parsing package: %s", filename)
	pkg, err := NewPackage(file, false)
	if err != nil {
		t.Fatalf("Failed to parse package %s: %v", filename, err)
	}

	// Verify we have metadata
	if len(pkg.Metadata) == 0 {
		t.Error("Package has no metadata")
	}

	// Log metadata
	t.Logf("Metadata entries: %d", len(pkg.Metadata))
	for metaType, value := range pkg.Metadata {
		t.Logf("  %v: %s", metaType, value)
	}

	// Verify we have file names mapping
	if len(pkg.FileNames) == 0 {
		t.Error("Package has no file names mapping")
	}
	t.Logf("File names mapping: %d entries", len(pkg.FileNames))

	// Verify we have file infos
	if len(pkg.FileInfos) == 0 {
		t.Error("Package has no file infos")
	}
	t.Logf("File infos: %d files", len(pkg.FileInfos))

	// Verify we have file contents
	if len(pkg.Files) == 0 {
		t.Error("Package has no files")
	}
	t.Logf("Files extracted: %d", len(pkg.Files))

	// Verify that the number of files matches
	if len(pkg.Files) != len(pkg.FileInfos) {
		t.Errorf("Mismatch: %d files extracted but %d file infos", len(pkg.Files), len(pkg.FileInfos))
	}

	// Check each file
	for fileName, content := range pkg.Files {
		fileInfo, exists := pkg.FileInfos[fileName]
		if !exists {
			t.Errorf("File %s has no corresponding FileInfo", fileName)
			continue
		}

		// Verify file size
		expectedSize := int(fileInfo.Size - 4)
		if len(content) != expectedSize {
			t.Errorf("File %s: size mismatch (got %d, expected %d)", fileName, len(content), expectedSize)
		}

		t.Logf("  -> %s (%d bytes)", fileName, len(content))
	}

	// Check required metadata fields
	requiredFields := []MetaType{Creator, BeatmapSetID}
	for _, field := range requiredFields {
		if _, exists := pkg.Metadata[field]; !exists {
			t.Errorf("Missing required metadata field: %v", field)
		}
	}
}

// TestMetadataOnly tests parsing with metadata only (without file contents)
func TestMetadataOnly(t *testing.T) {
	testFile := "tests/nekodex - welcome to christmas.osz2"

	// Check if file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatalf("Test file does not exist: %s", testFile)
	}

	// Open the file
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file %s: %v", testFile, err)
	}
	defer file.Close()

	// Parse with metadataOnly = true
	t.Logf("Parsing package with metadata only: %s", testFile)
	pkg, err := NewPackage(file, true)
	if err != nil {
		t.Fatalf("Failed to parse package %s: %v", testFile, err)
	}

	// Verify we have metadata
	if len(pkg.Metadata) == 0 {
		t.Error("Package has no metadata")
	}
	t.Logf("Metadata entries: %d", len(pkg.Metadata))

	// Verify we have file names mapping
	if len(pkg.FileNames) == 0 {
		t.Error("Package has no file names mapping")
	}
	t.Logf("File names mapping: %d entries", len(pkg.FileNames))

	// In metadata-only mode, we should not have file contents
	if len(pkg.Files) != 0 {
		t.Errorf("Expected no files in metadata-only mode, but got %d", len(pkg.Files))
	}

	// But we should still have the key generated for potential future use
	if len(pkg.key) == 0 {
		t.Error("Package key was not generated")
	}
}

// TestInvalidFile tests handling of invalid files
func TestInvalidFile(t *testing.T) {
	// Create a temporary invalid file
	tmpFile, err := os.CreateTemp("", "invalid-*.osz2")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write invalid data
	tmpFile.Write([]byte("This is not a valid osz2 file"))
	tmpFile.Seek(0, 0)

	// Try to parse it
	_, err = NewPackage(tmpFile, false)
	if err == nil {
		t.Error("Expected error when parsing invalid file, got nil")
	}

	t.Logf("Correctly rejected invalid file with error: %v", err)
}

// BenchmarkParsePackage benchmarks package parsing
func BenchmarkParsePackage(b *testing.B) {
	testFile := "tests/nekodex - welcome to christmas.osz2"

	// Check if file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		b.Skip("Test file does not exist")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		file, err := os.Open(testFile)
		if err != nil {
			b.Fatalf("Failed to open file: %v", err)
		}

		_, err = NewPackage(file, false)
		if err != nil {
			b.Fatalf("Failed to parse package: %v", err)
		}

		file.Close()
	}
}

// BenchmarkParseMetadataOnly benchmarks metadata-only parsing
func BenchmarkParseMetadataOnly(b *testing.B) {
	testFile := "tests/nekodex - welcome to christmas.osz2"

	// Check if file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		b.Skip("Test file does not exist")
		return
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		file, err := os.Open(testFile)
		if err != nil {
			b.Fatalf("Failed to open file: %v", err)
			return
		}

		_, err = NewPackage(file, true)
		if err != nil {
			b.Fatalf("Failed to parse package: %v", err)
			return
		}

		file.Close()
	}
}
