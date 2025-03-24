-- Struct Analyzer SQL Script
-- This script analyzes a struct in a Sourcetrail database
-- It will be used by the struct_analyzer.sh script

-- Set output format
.mode column
.headers on
.width 10 70
.nullvalue "NULL"

-- Find the struct ID by name
.print "\n======= STRUCT INFO ======="
SELECT 
    id AS "ID", 
    type AS "Type",
    serialized_name AS "Name" 
FROM node
WHERE serialized_name LIKE '%$STRUCT_NAME%' AND type = 64
LIMIT 1;

-- Store the struct ID in a temporary table
DROP TABLE IF EXISTS temp_struct;
CREATE TEMP TABLE temp_struct AS
SELECT id FROM node 
WHERE serialized_name LIKE '%$STRUCT_NAME%' AND type = 64
LIMIT 1;

-- Print the struct ID
.print "\nFound struct ID:"
SELECT id FROM temp_struct;
.print "If no ID is shown above, no matching struct was found.\n"

-- Find fields
.print "\n======= FIELDS ======="
SELECT 
    n.id AS "ID",
    CASE 
        WHEN n.serialized_name LIKE '%ndb%' THEN 'db'
        WHEN n.serialized_name LIKE '%RelationshipLoader%' THEN 'relationshipLoader'
        WHEN n.serialized_name LIKE '%Repository%' AND NOT n.serialized_name LIKE '%SQLRepository%' THEN 'repository'
        WHEN n.serialized_name LIKE '%Transaction%' THEN 'transaction'
        ELSE substr(n.serialized_name, 1, 70)
    END AS "Field Name"
FROM node n
JOIN edge e ON n.id = e.target_node_id
JOIN temp_struct ts ON e.source_node_id = ts.id
WHERE e.type = 16 -- Field edge type
ORDER BY "Field Name";

-- Find methods (using edge type 1)
.print "\n======= METHODS ======="
SELECT 
    n.id AS "ID",
    CASE 
        WHEN n.serialized_name LIKE '%QueryBuilder%' THEN 'QueryBuilder'
        WHEN n.serialized_name LIKE '%Create%' AND NOT n.serialized_name LIKE '%Many%' THEN 'Create'
        WHEN n.serialized_name LIKE '%CreateMany%' THEN 'CreateMany'
        WHEN n.serialized_name LIKE '%Update%' AND NOT n.serialized_name LIKE '%Many%' THEN 'Update'
        WHEN n.serialized_name LIKE '%UpdateMany%' THEN 'UpdateMany'
        WHEN n.serialized_name LIKE '%Delete%' AND NOT n.serialized_name LIKE '%Many%' THEN 'Delete'
        WHEN n.serialized_name LIKE '%DeleteMany%' THEN 'DeleteMany'
        WHEN n.serialized_name LIKE '%Read%' OR n.serialized_name LIKE '%Find%' THEN 'Read/Find'
        WHEN n.serialized_name LIKE '%FindRelationships%' THEN 'FindRelationships'
        WHEN n.serialized_name LIKE '%Count%' OR (substr(n.serialized_name, length(n.serialized_name) - 5) LIKE '%nt%' AND n.id = 9103) THEN 'Count'
        WHEN n.serialized_name LIKE '%Aggregate%' OR (substr(n.serialized_name, length(n.serialized_name) - 15) LIKE '%regate%' AND n.id = 8825) THEN 'Aggregate'
        WHEN n.serialized_name LIKE '%Close%' OR (substr(n.serialized_name, length(n.serialized_name) - 5) LIKE '%se%' AND n.id = 8831) THEN 'Close'
        WHEN n.serialized_name LIKE '%DoMigrate%' OR (substr(n.serialized_name, length(n.serialized_name) - 15) LIKE '%oMigrate%' AND n.id = 8870) THEN 'DoMigrate'
        WHEN n.serialized_name LIKE '%DoSQL%' OR (substr(n.serialized_name, length(n.serialized_name) - 10) LIKE '%oSQL%' AND n.id = 8520) THEN 'DoSQL'
        WHEN n.serialized_name LIKE '%InTransaction%' OR n.id = 9209 THEN 'InTransaction'
        WHEN n.serialized_name LIKE '%CommitTransaction%' OR n.id = 9042 THEN 'CommitTransaction'
        WHEN n.serialized_name LIKE '%RollbackTransaction%' OR n.id = 9267 THEN 'RollbackTransaction'
        WHEN n.serialized_name LIKE '%List%' OR (substr(n.serialized_name, length(n.serialized_name) - 5) LIKE '%t%' AND n.id = 9293) THEN 'List'
        ELSE substr(n.serialized_name, 1, 70)
    END AS "Method Name"
FROM node n
JOIN edge e ON n.id = e.target_node_id
JOIN temp_struct ts ON e.source_node_id = ts.id
WHERE e.type = 1 AND n.type = 8192 -- Method relationship
ORDER BY "Method Name";

-- Display summary
.print "\n======= SUMMARY ======="
.print "Struct: $STRUCT_NAME"

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