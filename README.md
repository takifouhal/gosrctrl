# GoSrcCtrl

A Go source code parser and indexer for Sourcetrail.

## Overview

GoSrcCtrl parses Go source code and generates output that can be imported into Sourcetrail for visualization and navigation. It consists of two main components:

1. A Go parser that analyzes Go source code and outputs structural information in JSON format
2. A Python indexer that processes the JSON output and creates a Sourcetrail database

## Requirements

- Go 1.18+ 
- Python 3.8+
- Make (for automated setup)

## Setup

1. Clone this repository:

```bash
git clone <repository-url>
cd gosrctrl
```

2. Automated setup with Make:

```bash
# Setup both Go and Python environments (recommended)
make

# For help with other make commands
make help
```

3. Manual setup (alternative to Make):

   a. Set up Go dependencies:

   ```bash
   go get golang.org/x/tools/go/packages
   ```

   b. Set up Python virtual environment:

   ```bash
   # Create and activate virtual environment
   python3 -m venv venv
   source venv/bin/activate  # On Windows, use: venv\Scripts\activate

   # Install required Python packages
   pip install -r requirements.txt
   ```

## VSCode Integration

The project includes VSCode settings that will automatically use the virtual environment. After opening the project in VSCode:

1. Open any Python file
2. VSCode should prompt to select the Python interpreter - choose the one from the venv directory
3. VSCode will now use the virtual environment for running and debugging Python code

## Usage

(To be implemented)

In the future, the tool will be used as follows:

1. Parse Go source code with the Go parser:

```bash
go run main.go -src /path/to/go/project -out output.json
```

2. Process the output and create a Sourcetrail database:

```bash
# Make sure the virtual environment is activated
source venv/bin/activate  # On Windows, use: venv\Scripts\activate

python3 indexer.py -i output.json -o output.srctrldb
```

## Project Status

This project is currently in the initial setup phase. Stay tuned for updates. 