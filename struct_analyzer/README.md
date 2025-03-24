# Struct Analyzer for Sourcetrail DB

This tool analyzes Go structs in a Sourcetrail database and displays detailed information about their fields and methods. It uses a generic approach to extract field and method names without relying on any hardcoded values, ensuring it works with any codebase.

## Features

- Identifies structs in a Sourcetrail database by name
- Extracts field and method names generically
- Provides a summary count of fields and methods
- Supports saving output to a file
- Works with any Go struct in any codebase

## Usage

```bash
./struct_analyzer.sh <database_path> <struct_name> [options]
```

### Options

- `-h, --help`: Show help message
- `-o, --output <file>`: Save output to a file

### Examples

```bash
# Display analysis in terminal
./struct_analyzer.sh ~/path/to/project.srctrldb SQLRepository

# Save analysis to a file
./struct_analyzer.sh ~/path/to/project.srctrldb SQLRepository -o sql_repo_info.txt

# Analyze a different struct
./struct_analyzer.sh ~/path/to/project.srctrldb HttpHandler
```

## Output

The tool provides:

1. **Struct Info**: Basic information about the struct, including its ID and serialized name
2. **Fields**: List of the struct's fields with their IDs and names
3. **Methods**: List of the struct's methods with their IDs and names
4. **Summary**: A count of fields and methods

## Requirements

- SQLite3
- Bash

## Files

- `struct_analyzer.sh` - Main shell script
- `struct_analyzer.sql` - SQL queries for analyzing the struct

## Notes

- The tool works with any Sourcetrail database containing Go code
- No hardcoded values are used, ensuring flexibility with any codebase
- Color formatting is used for terminal output but is stripped when saving to a file 