# GoSrcCtrl

A Go source code parser and indexer for Sourcetrail.

## Overview

GoSrcCtrl parses Go source code and generates output that can be imported into Sourcetrail for visualization and navigation. It consists of two main components:

1. A Go parser that analyzes Go source code and outputs structural information in JSON format
2. A Python indexer that processes the JSON output and creates a Sourcetrail database

## Requirements

- Go 1.18+ 
- Python 3.8+
- Numbat (`pip3 install numbat`)

## Setup

1. Clone this repository:

```
git clone <repository-url>
cd gosrctrl
```

2. Ensure dependencies are installed:

```
go get golang.org/x/tools/go/packages
pip3 install numbat
```

## Usage

(To be implemented)

In the future, the tool will be used as follows:

1. Parse Go source code with the Go parser:

```
go run main.go -src /path/to/go/project -out output.json
```

2. Process the output and create a Sourcetrail database:

```
python3 indexer.py -i output.json -o output.srctrldb
```

## Project Status

This project is currently in the initial setup phase. Stay tuned for updates. 