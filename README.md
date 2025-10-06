# osz2-go

[![Go Version](https://img.shields.io/github/go-mod/go-version/Lekuruu/osz2-go)](https://go.dev/)
[![Go Reference](https://pkg.go.dev/badge/github.com/Lekuruu/osz2-go.svg)](https://pkg.go.dev/github.com/Lekuruu/osz2-go)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Lekuruu/osz2-go/.github%2Fworkflows%2Fbuild.yml)](https://github.com/Lekuruu/osz2-go/actions/workflows/build.yml)
[![GitHub License](https://img.shields.io/github/license/Lekuruu/osz2-go)](https://github.com/Lekuruu/osz2-go/blob/main/LICENSE)

osz2-go is a go library for reading and extracting osz2 files. It provides functionality to parse, decrypt, and extract osz2 beatmap packages, using [Osz2Decryptor](https://github.com/xxCherry/Osz2Decryptor) by [xxCherry](https://github.com/xxCherry) as a reference.

## Features

- Parse osz2 package files
    - Extract metadata (artist, title, difficulty, etc.)
    - Decrypt XXTEA-encrypted content
    - Extract all files from the package, including file info
- Command-line interface for easy extraction

## Usage

This repository provides a separate CLI application, which you can use to extract osz2 packages. View the [readme file](https://github.com/Lekuruu/osz2-go/blob/main/cmd/cli/README.md) to learn more.

Here is an example of how to use osz2-go as a library:

```bash
go get github.com/Lekuruu/osz2-go
```

```go
package main

import (
    "fmt"
    "os"
    "github.com/Lekuruu/osz2-go"
)

func main() {
    file, err := os.Open("beatmap.osz2")
    if err != nil {
        panic(err)
    }
    defer file.Close()

    // Parse package (metadataOnly: false to read all files)
    pkg, err := osz2.NewPackage(file, false)
    if err != nil {
        panic(err)
    }

    // Access metadata
    fmt.Println("Title:", pkg.Metadata[osz2.MetaTitle])
    fmt.Println("Artist:", pkg.Metadata[osz2.MetaArtist])

    // Access files
    for filename, content := range pkg.Files {
        fmt.Printf("File: %s, Size: %d bytes\n", filename, len(content))
    }
}
```
