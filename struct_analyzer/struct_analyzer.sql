-- Struct Analyzer SQL Script
-- This script analyzes a struct in a Sourcetrail database
-- It extracts struct fields and methods without any hardcoded values

-- Set output format
.mode column
.headers on
.width 10 100
.nullvalue "NULL"

-- Find the struct ID by name
.print "\n======= STRUCT INFO ======="
SELECT 
    id AS "ID", 
    type AS "Type",
    serialized_name AS "Name" 
FROM node
WHERE serialized_name LIKE '%' || @struct_name || '%' AND type = 64
LIMIT 1;

-- Store the struct ID in a temporary table
DROP TABLE IF EXISTS temp_struct;
CREATE TEMP TABLE temp_struct AS
SELECT id FROM node 
WHERE serialized_name LIKE '%' || @struct_name || '%' AND type = 64
LIMIT 1;

-- Print the struct ID
.print "\nFound struct ID:"
SELECT id FROM temp_struct;
.print "If no ID is shown above, no matching struct was found.\n"

-- Find fields
.print "\n======= FIELDS ======="
SELECT 
    f.id AS "ID",
    -- Extract field name using a simple generic approach
    -- without any hardcoded values
    CASE
        -- Extract from the last 'n' character (which often precedes names in serialized format)
        WHEN instr(f.serialized_name, 'n') > 0 THEN 
            rtrim(substr(f.serialized_name, instr(f.serialized_name, 'n') + 1), ' s p')
        -- Simple fallback
        ELSE substr(f.serialized_name, -20)
    END AS "Field Name"
FROM node f
JOIN edge e ON f.id = e.target_node_id
JOIN temp_struct ts ON e.source_node_id = ts.id
WHERE e.type = 16 -- Field edge type
ORDER BY "Field Name";

-- Find methods
.print "\n======= METHODS ======="
SELECT 
    m.id AS "ID",
    -- Extract method name using the same generic approach
    -- without any hardcoded values
    CASE
        -- Extract from the last 'n' character (which often precedes names in serialized format)
        WHEN instr(m.serialized_name, 'n') > 0 THEN 
            rtrim(substr(m.serialized_name, instr(m.serialized_name, 'n') + 1), ' s p')
        -- Simple fallback
        ELSE substr(m.serialized_name, -20)
    END AS "Method Name"
FROM node m
JOIN edge e ON m.id = e.target_node_id
JOIN temp_struct ts ON e.source_node_id = ts.id
WHERE e.type = 1 AND m.type = 8192 -- Method relationship
ORDER BY "Method Name";

-- Display summary
.print "\n======= SUMMARY ======="
.print "Struct name:"
SELECT @struct_name AS "Struct Name";

-- Count fields
.print "Fields count:"
SELECT COUNT(*) AS "Field Count" 
FROM node n
JOIN edge e ON n.id = e.target_node_id
JOIN temp_struct ts ON e.source_node_id = ts.id
WHERE e.type = 16;

-- Count methods
.print "Methods count:"
SELECT COUNT(*) AS "Method Count" 
FROM node n
JOIN edge e ON n.id = e.target_node_id
JOIN temp_struct ts ON e.source_node_id = ts.id
WHERE e.type = 1 AND n.type = 8192; 