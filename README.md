# osz2-go

[![Go Version](https://img.shields.io/github/go-mod/go-version/Lekuruu/osz2-go)](https://go.dev/)
[![Go Reference](https://pkg.go.dev/badge/github.com/Lekuruu/osz2-go.svg)](https://pkg.go.dev/github.com/Lekuruu/osz2-go)
[![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Lekuruu/osz2-go/.github%2Fworkflows%2Fbuild.yml)](https://github.com/Lekuruu/osz2-go/actions/workflows/build.yml)
[![GitHub License](https://img.shields.io/github/license/Lekuruu/osz2-go)](https://github.com/Lekuruu/osz2-go/blob/main/LICENSE)

osz2-go is a Go library for reading <!-- & writing --> `.osz2` and `.osf2` packages. The format work uses [Osz2Decryptor](https://github.com/xxCherry/Osz2Decryptor) by [xxCherry](https://github.com/xxCherry) as a reference.

A package `Reader` is just a standard [`fs.FS`](https://pkg.go.dev/io/fs#FS), with the extra metadata that osz2 packages provide. File bodies are being decrypted while they are read, instead of being loaded into memory all at once, which is pretty cool. Writing packages through `fs.FS` interfaces is also planned for the near future.

This repository also provides a separate CLI application for extracting osz2 / osf2 packages. View the [readme file](cmd/cli/README.md) for usage instructions.

## Usage

Here is an example of how to use osz2-go as a library:

```bash
go get github.com/Lekuruu/osz2-go
```

```go
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/Lekuruu/osz2-go"
)

func main() {
	// Opening the package will return a corresponding filesystem reader
	// You can also use `osz2.NewReader(...)`, which cannot be Close()'d however
	reader, err := osz2.OpenReader("beatmap.osz2", osz2.KeyTypeOsz2)
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	// Access metadata
	metadata := reader.Metadata()
	fmt.Println("Title:", metadata[osz2.Title])
	fmt.Println("Artist:", metadata[osz2.Artist])

	// Standard io/fs helpers work as you'd expect
	err = fs.WalkDir(reader, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		fmt.Println(name)
		return nil
	})
	if err != nil {
		panic(err)
	}

	// Open returns a fs.File interface that will
	// decrypt the file on the fly as its being read
	// You may also use `OpenEntry` to get
	// access to a few more functions
	file, err := reader.Open("audio.mp3")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// idk, do whatever with this
	// e.g. read a few bytes
	header := make([]byte, 32)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		panic(err)
	}

	fmt.Printf("Audio header bytes: %x\n", header[:n])
}
```

Use `KeyTypeOsf2` instead when opening an osu!stream `.osf2` package.

<!-- TODO: writing osz2 files -->
