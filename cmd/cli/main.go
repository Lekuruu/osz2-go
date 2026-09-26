package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Lekuruu/osz2-go"
)

func main() {
	inputFile := flag.String("input", "", "Path to the .osz2 file (required)")
	outputDir := flag.String("output", "", "Output directory for extracted files (required)")
	metadataFile := flag.String("metadata", "metadata.json", "Output path for metadata JSON file")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Show help if requested or if required flags are missing
	if *help || *inputFile == "" || *outputDir == "" {
		printHelp()
		os.Exit(0)
	}

	// Check if input file exists
	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Input file does not exist: %s\n", *inputFile)
		os.Exit(1)
	}

	// Open the osz2 file
	file, err := os.Open(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// Parse the osz2 package (metadataOnly => false to read all files)
	fmt.Println("Reading osz2 package...")
	pkg, err := osz2.NewPackage(file, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing osz2 package: %v\n", err)
		os.Exit(1)
	}

	metadataPath, err := resolveMetadataPath(*outputDir, *metadataFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error validating metadata path: %v\n", err)
		os.Exit(1)
	}

	// Extract files
	files := pkg.Files()
	fmt.Printf("Extracting %d files to %s...\n", len(files), *outputDir)

	if err := extractFiles(*outputDir, files); err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting files: %v\n", err)
		os.Exit(1)
	}
	for fileName, content := range files {
		fmt.Printf("  -> %s (%d bytes)\n", fileName, len(content))
	}

	// Build metadata structure
	metadata := buildMetadata(pkg)

	// Write metadata to JSON file
	jsonData, err := json.MarshalIndent(metadata, "", "    ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling metadata to JSON: %v\n", err)
		os.Exit(1)
	}

	if err := writeMetadataFile(*outputDir, *metadataFile, jsonData); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing metadata file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nExtraction complete!\n")
	fmt.Printf("  Files extracted: %d\n", len(files))
	fmt.Printf("  Metadata saved to: %s\n", metadataPath)
}

func printHelp() {
	fmt.Println("osz2 Extractor - Extract .osz2 files and save metadata")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  osz2-cli -input <file.osz2> -output <directory> [-metadata <metadata.json>]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -input string")
	fmt.Println("        Path to the .osz2 file (required)")
	fmt.Println("  -output string")
	fmt.Println("        Output directory for extracted files (required)")
	fmt.Println("  -metadata string")
	fmt.Println("        Output path for metadata JSON file (default: metadata.json in output directory)")
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  osz2-cli -input beatmap.osz2 -output ./extracted")
	fmt.Println("  osz2-cli -input beatmap.osz2 -output ./extracted -metadata info.json")
}

func extractFiles(outputDir string, files map[string][]byte) error {
	targets := make(map[string][]byte, len(files))
	for fileName, content := range files {
		target, err := normalizeExtractionPath(fileName)
		if err != nil {
			return err
		}
		if _, exists := targets[target]; exists {
			return fmt.Errorf("multiple package files resolve to %q", target)
		}
		targets[target] = content
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return fmt.Errorf("open output directory: %w", err)
	}
	defer root.Close()

	for target, content := range targets {
		if dir := path.Dir(target); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create directory for %q: %w", target, err)
			}
		}
		if err := root.WriteFile(target, content, 0o644); err != nil {
			return fmt.Errorf("write %q: %w", target, err)
		}
	}
	return nil
}

func normalizeExtractionPath(fileName string) (string, error) {
	normalized := strings.ReplaceAll(fileName, `\`, "/")
	if normalized == "" || strings.ContainsRune(normalized, 0) || path.IsAbs(normalized) || hasWindowsDrivePrefix(normalized) {
		return "", fmt.Errorf("unsafe package path %q", fileName)
	}

	target := path.Clean(normalized)
	if target == "." || !fs.ValidPath(target) {
		return "", fmt.Errorf("unsafe package path %q", fileName)
	}
	return target, nil
}

func resolveMetadataPath(outputDir, metadataPath string) (string, error) {
	if filepath.IsAbs(metadataPath) {
		return filepath.Clean(metadataPath), nil
	}
	target, err := normalizeExtractionPath(metadataPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(outputDir, filepath.FromSlash(target)), nil
}

func writeMetadataFile(outputDir, metadataPath string, data []byte) error {
	if filepath.IsAbs(metadataPath) {
		return os.WriteFile(metadataPath, data, 0o644)
	}
	target, err := normalizeExtractionPath(metadataPath)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if dir := path.Dir(target); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return root.WriteFile(target, data, 0o644)
}

func hasWindowsDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	return name[0] >= 'a' && name[0] <= 'z' || name[0] >= 'A' && name[0] <= 'Z'
}
