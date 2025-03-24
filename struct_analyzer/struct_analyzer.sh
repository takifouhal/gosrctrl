#!/bin/bash
# Struct Analyzer - A tool to analyze Go structs in Sourcetrail databases

# Set colors for better output formatting
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Display help
show_help() {
    echo -e "${BLUE}Struct Analyzer for Sourcetrail DB${NC}"
    echo -e "Analyzes Go structs in a Sourcetrail database and shows fields and methods"
    echo
    echo -e "${YELLOW}Usage: $0 <database_path> <struct_name> [options]${NC}"
    echo
    echo -e "Options:"
    echo -e "  -h, --help     Show this help message"
    echo -e "  -o, --output   Save output to a file (e.g., -o output.txt)"
    echo
    echo -e "Example:"
    echo -e "  $0 /path/to/sourcetrail.srctrldb SQLRepository"
    echo -e "  $0 /path/to/sourcetrail.srctrldb SQLRepository -o sql_repo_info.txt"
    echo
}

# Parse options
OUTPUT_FILE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -o|--output)
            if [[ -z "$2" || "$2" == -* ]]; then
                echo -e "${RED}Error: -o option requires a filename argument${NC}"
                exit 1
            fi
            OUTPUT_FILE="$2"
            shift 2
            ;;
        *)
            # Store positional arguments
            if [ -z "$DB_PATH" ]; then
                DB_PATH="$1"
            elif [ -z "$STRUCT_NAME" ]; then
                STRUCT_NAME="$1"
            else
                echo -e "${RED}Error: Unexpected argument: $1${NC}"
                show_help
                exit 1
            fi
            shift
            ;;
    esac
done

# Check if database path is provided
if [ -z "$DB_PATH" ]; then
    echo -e "${RED}Error: Missing database path.${NC}"
    show_help
    exit 1
fi

# Check if struct name is provided
if [ -z "$STRUCT_NAME" ]; then
    echo -e "${RED}Error: Missing struct name.${NC}"
    show_help
    exit 1
fi

# Check if database file exists
if [ ! -f "$DB_PATH" ]; then
    echo -e "${RED}Error: Database file not found: $DB_PATH${NC}"
    exit 1
fi

# Header output
header="${BLUE}=====================================${NC}
${GREEN}Analyzing struct: ${YELLOW}$STRUCT_NAME${GREEN} in database: ${YELLOW}$DB_PATH${NC}
${BLUE}=====================================${NC}"

echo -e "$header"

# Create a temporary SQL file with the struct name directly embedded
TMP_SQL_FILE=$(mktemp)
cat "$SCRIPT_DIR/struct_analyzer.sql" | sed "s/@struct_name/'$STRUCT_NAME'/g" | sed "s/|| @struct_name/|| '$STRUCT_NAME'/g" > "$TMP_SQL_FILE"

# Run the SQL script
if [ -n "$OUTPUT_FILE" ]; then
    # Save to file, strip color codes for file output
    (echo "Analyzing struct: $STRUCT_NAME in database: $DB_PATH"; echo; sqlite3 "$DB_PATH" < "$TMP_SQL_FILE") > "$OUTPUT_FILE"
    RESULT=$?
    
    if [ $RESULT -eq 0 ]; then
        echo -e "${GREEN}Output saved to: ${YELLOW}$OUTPUT_FILE${NC}"
    else
        echo -e "${RED}Error while saving output to: $OUTPUT_FILE${NC}"
    fi
else
    # Display in terminal
    sqlite3 "$DB_PATH" < "$TMP_SQL_FILE"
    RESULT=$?
fi

# Clean up
rm "$TMP_SQL_FILE"

# Exit with SQLite's exit code
exit $RESULT 