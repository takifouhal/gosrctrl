# Go Struct Analyzer for Sourcetrail

This tool analyzes Go structs in a Sourcetrail database using dynamic pattern recognition to extract field and method information.

## Features

- Extracts field information using pattern recognition instead of hardcoded symbol names
- Dynamically derives method names based on common patterns and fallback strategies
- Works with any Go struct in any codebase
- No hardcoded dependencies on specific symbol names or formats
- Outputs a clean summary for easy review

## Usage

```bash
./struct_analyzer.sh <path_to_srctrldb_file> <struct_name>
```

### Example

```bash
# Analyze SQLRepository struct
./struct_analyzer.sh /path/to/your/database.srctrldb SQLRepository

# Save the output to a file
./struct_analyzer.sh /path/to/your/database.srctrldb SQLRepository > analysis_results.txt
```

## Path Considerations When Generating Sourcetrail DB

When generating the Sourcetrail database with `gosrctrl`, it's recommended to:

1. Always use absolute paths with the `-path` parameter:
   ```bash
   # This works consistently
   gosrctrl -path $(pwd) -out db_file.srctrldb
   
   # Or full explicit path
   gosrctrl -path /Users/username/path/to/project -out db_file.srctrldb
   ```

2. Avoid using relative paths which may cause errors:
   ```bash
   # This might cause errors
   gosrctrl -path . -out db_file.srctrldb
   ```

## Output

The analyzer outputs the following information:

1. **Struct Info**: Basic information about the struct, including its ID
2. **Fields**: List of fields belonging to the struct, with their names and IDs
3. **Methods**: List of methods belonging to the struct, with their names and IDs
4. **Summary**: A count of fields and methods

## How It Works

The analyzer uses SQLite queries to extract information from the Sourcetrail database. It identifies fields and methods by:

1. Finding the struct ID by name
2. Extracting fields associated with the struct
3. Extracting methods associated with the struct
4. Using pattern recognition to derive meaningful names

This tool works without hardcoded symbol names, making it adaptable to any Go codebase.

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