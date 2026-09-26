package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Lekuruu/osz2-go/pkg/osz2"
)

func main() {
	os.Exit(run())
}

func run() int {
	inputFile := flag.String("input", "", "Path to the .osz2 or .osf2 file (required)")
	outputDir := flag.String("output", "", "Output directory for extracted files (required)")
	metadataFile := flag.String("metadata", "metadata.json", "Output path for metadata JSON file")
	help := flag.Bool("help", false, "Show help message")
	flag.Parse()

	// Show help if requested or if required flags are missing
	if *help || *inputFile == "" || *outputDir == "" {
		printHelp()
		return 0
	}

	// Pick the encryption scheme from the file extension
	var keyType osz2.KeyType
	switch strings.ToLower(filepath.Ext(*inputFile)) {
	case ".osz2":
		keyType = osz2.KeyTypeOsz2
	case ".osf2":
		keyType = osz2.KeyTypeOsf2
	default:
		fmt.Fprintln(os.Stderr, "Error: input must be an .osz2 or .osf2 file")
		return 1
	}

	// Mount the package
	fmt.Printf("Reading %s package...\n", keyType)
	reader, err := osz2.OpenReader(*inputFile, keyType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing package: %v\n", err)
		return 1
	}
	defer reader.Close()

	// Write out the metadata first before we extract any files
	metadata, err := buildMetadata(reader.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building metadata: %v\n", err)
		return 1
	}
	jsonData, err := json.MarshalIndent(metadata, "", "    ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling metadata to JSON: %v\n", err)
		return 1
	}

	metadataPath, err := writeMetadataFile(*outputDir, *metadataFile, jsonData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing metadata file: %v\n", err)
		return 1
	}
	fmt.Printf("Wrote metadata to %s.\n", metadataPath)

	// Extract files one at a time so large packages do not need to fit in memory
	fmt.Printf("Extracting files to %s...\n", *outputDir)
	fileCount, err := extractFiles(*outputDir, reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting files: %v\n", err)
		return 1
	}

	fmt.Printf("\nExtraction complete!\n")
	fmt.Printf("  Files extracted: %d\n", fileCount)
	fmt.Printf("  Metadata saved to: %s\n", metadataPath)
	return 0
}

func printHelp() {
	fmt.Println("osz2 Extractor - Extract .osz2 / .osf2 files and save metadata")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  osz2-cli -input <file.osz2|file.osf2> -output <directory> [-metadata <metadata.json>]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -input string")
	fmt.Println("        Path to the .osz2 / .osf2 file (required)")
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

func extractFiles(outputDir string, source fs.FS) (int, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return 0, fmt.Errorf("create output directory: %w", err)
	}
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return 0, fmt.Errorf("open output directory: %w", err)
	}
	defer root.Close()

	fileCount := 0
	err = fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target, err := normalizeExtractionPath(name)
		if err != nil {
			return err
		}
		if dir := path.Dir(target); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create directory for %q: %w", target, err)
			}
		}

		input, err := source.Open(name)
		if err != nil {
			return err
		}
		output, err := root.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			input.Close()
			return fmt.Errorf("open %q for writing: %w", target, err)
		}

		_, copyErr := io.Copy(output, input)
		outputCloseErr := output.Close()
		inputCloseErr := input.Close()
		if copyErr != nil {
			return fmt.Errorf("write %q: %w", target, copyErr)
		}
		if outputCloseErr != nil {
			return fmt.Errorf("close %q: %w", target, outputCloseErr)
		}
		if inputCloseErr != nil {
			return fmt.Errorf("close package file %q: %w", name, inputCloseErr)
		}

		fileCount++
		fmt.Printf("  -> %s (%d bytes)\n", name, info.Size())
		return nil
	})
	return fileCount, err
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

func writeMetadataFile(outputDir, metadataPath string, data []byte) (string, error) {
	if filepath.IsAbs(metadataPath) {
		target := filepath.Clean(metadataPath)
		return target, os.WriteFile(target, data, 0o644)
	}

	target, err := normalizeExtractionPath(metadataPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(outputDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if dir := path.Dir(target); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := root.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	return filepath.Join(outputDir, filepath.FromSlash(target)), nil
}

func hasWindowsDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	return name[0] >= 'a' && name[0] <= 'z' || name[0] >= 'A' && name[0] <= 'Z'
}
