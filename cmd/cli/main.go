package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Extract files
	fmt.Printf("Extracting %d files to %s...\n", len(pkg.Files), *outputDir)
	for fileName, content := range pkg.Files {
		outputPath := filepath.Join(*outputDir, fileName)

		// Create subdirectories if needed
		if dir := filepath.Dir(outputPath); dir != "." {
			if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory for %s: %v\n", fileName, err)
				continue
			}
		}

		// Write file
		if err := os.WriteFile(outputPath, content, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file %s: %v\n", fileName, err)
			continue
		}
		fmt.Printf("  ✓ %s (%d bytes)\n", fileName, len(content))
	}

	// Build metadata structure
	metadata := buildMetadata(pkg)

	// Write metadata to JSON file
	metadataPath := *metadataFile
	if !filepath.IsAbs(metadataPath) {
		metadataPath = filepath.Join(*outputDir, metadataPath)
	}

	jsonData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling metadata to JSON: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(metadataPath, jsonData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing metadata file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Extraction complete!\n")
	fmt.Printf("  Files extracted: %d\n", len(pkg.Files))
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
