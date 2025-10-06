# osz2-cli tool

A command-line interface for extracting `.osz2` files and saving their metadata in a json format.

## Building

```bash
go build -o osz2-cli ./cmd/cli/
```

## Usage

```bash
osz2-cli -input <file.osz2> -output <directory> [-metadata <metadata.json>]
```

### Flags

- `-help` Show help message
- `-input` (required): Path to the `.osz2` file to extract
- `-output` (required): Output directory where files will be extracted
- `-metadata` (optional): Path for the metadata JSON file (default: `metadata.json` in the output directory)

### Examples

Extract a beatmap to a directory:

```bash
osz2-cli -input "nekodex - welcome to christmas.osz2" -output ./extracted
```

Extract with a custom metadata file path:

```bash
osz2-cli -input beatmap.osz2 -output ./my_beatmap -metadata beatmap_info.json
```
