#!/bin/bash
#
# Struct Analyzer for Sourcetrail DB
# This script analyzes a Go struct in a Sourcetrail database and displays its fields and methods
#
# Usage: ./struct_analyzer.sh <database_path> <struct_name>
# Example: ./struct_analyzer.sh /path/to/project.srctrldb SQLRepository

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Check if a database path was provided
if [ $# -lt 1 ]; then
    echo "Usage: $0 <database_path> <struct_name>"
    echo "Example: $0 ./my_project.srctrldb SQLRepository"
    exit 1
fi

# Check if a struct name was provided
if [ $# -lt 2 ]; then
    echo "Error: Please provide a struct name as the second argument."
    echo "Usage: $0 <database_path> <struct_name>"
    exit 1
fi

DB_PATH="$1"
STRUCT_NAME="$2"

# Check if the database file exists
if [ ! -f "$DB_PATH" ]; then
    echo "Error: Database file '$DB_PATH' does not exist."
    exit 1
fi

echo "Analyzing struct: $STRUCT_NAME in database: $DB_PATH"

# Create a temporary SQL file with the struct name embedded
TMP_SQL_FILE=$(mktemp)

cat > "$TMP_SQL_FILE" << EOF
$(cat "$SCRIPT_DIR/struct_analyzer.sql")
EOF

# Run the SQL script
sqlite3 "$DB_PATH" -cmd "PRAGMA temp_store_directory='$TMPDIR'" -cmd "PRAGMA foreign_keys=ON" < <(sed "s/\$STRUCT_NAME/$STRUCT_NAME/g" "$TMP_SQL_FILE")
RESULT=$?

# Clean up the temporary file
rm "$TMP_SQL_FILE"

# Exit with SQLite's exit code
exit $RESULT 