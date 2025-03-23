# GoSrcCtrl

A Go source code parser and indexer for Sourcetrail.

## Overview

GoSrcCtrl parses Go source code and generates output that can be imported into Sourcetrail for visualization and navigation. It consists of two main components:

1. A Go parser that analyzes Go source code and outputs structural information (symbols and references).
2. A Python script (`generate_db.py`) that processes the JSON output and creates a Sourcetrail database.

## Requirements

- Go 1.18+ 
- Python 3.8+
- Make (optional, for automated setup)

## Building & Installation

### Via Make

1. **Build locally**:
   ```bash
   make build
   ```
   This compiles the Go code into a binary named `gosrctrl` in the current directory. You can then run `./gosrctrl` from this folder.

2. **Install globally**:
   ```bash
   make install
   ```
   This places the binary into your `$GOPATH/bin` or `$GOBIN` directory. Ensure that directory is in your `$PATH`. You can then run `gosrctrl` from anywhere.

### Via Go Commands (Manual)

If you prefer not to use Make, you can build and install with standard Go tools:

```bash
# Build locally
go build -o gosrctrl main.go

# (Optional) Install globally
go install
```

Then verify installation:
```bash
which gosrctrl
gosrctrl -help
```

## Setup with Make

1. Clone this repository:
   ```bash
   git clone <repository-url>
   cd gosrctrl
   ```
2. Run:
   ```bash
   make
   ```
   This sets up Go dependencies and a Python virtual environment (in a `venv` folder).

## Usage

Once the `gosrctrl` binary is on your PATH (or you are in the same directory if built locally), you can run:

```bash
gosrctrl -path /path/to/go/project -out output.srctrldb
```

**Flags:**
- `-path` (default: current directory) – The path to the Go project you want to parse.
- `-out` (default: `output.srctrldb`) – Output file name (Sourcetrail database). If you provide a name without `.srctrldb`, it will be appended automatically.
- `-keepjson` (default: false) – If set to true, keeps the intermediate JSON file (otherwise it is removed after database creation).

**What happens under the hood?**
1. The Go code loads and parses the Go packages in the specified `-path`.
2. Symbols and references are extracted.
3. A JSON file is created with this information.
4. The script automatically calls `generate_db.py` to create the `.srctrldb` file from the JSON.
5. By default, the JSON file is removed unless `-keepjson` is specified.

### Manual Python usage (optional)

If you prefer to manually use the Python script (for debugging or customization), you can do:

```bash
# Generate the JSON file manually or via gosrctrl (with -keepjson)
python3 generate_db.py --input your_output.json --output final.srctrldb
```

## VSCode Integration

The project includes VSCode settings that will automatically use the Python virtual environment. After opening the project in VSCode:

1. Open any Python file.
2. VSCode should prompt to select the Python interpreter - choose the one from the venv directory.
3. VSCode will now use the virtual environment for running and debugging Python code.

## Project Status

This project is currently in the initial setup phase, but can parse a Go codebase and create a basic Sourcetrail database. Future enhancements will further refine references (e.g., calls vs. reads) and expand the Python scripting capabilities.

Stay tuned for updates.