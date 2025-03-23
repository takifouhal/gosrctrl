#!/usr/bin/env python3

"""
generate_db.py

Reads the JSON output from the Go parser (parser + export pipeline)
and creates a Sourcetrail DB (.srctrldb) using the Numbat library.

Usage:
    python generate_db.py --input path/to/output.json --output path/to/result.srctrldb
"""

import argparse
import json
import os
import re

from pathlib import Path
from numbat import SourcetrailDB

def main():
    parser = argparse.ArgumentParser(description="Generate a Sourcetrail DB from GoSrcCtrl JSON output using Numbat.")
    parser.add_argument("-i", "--input", required=True, help="Path to the JSON file with symbols and references.")
    parser.add_argument("-o", "--output", required=True, help="Path to the output .srctrldb file.")
    args = parser.parse_args()

    input_path = Path(args.input)
    output_path = Path(args.output)

    if not input_path.exists():
        print(f"Error: Input file does not exist: {input_path}")
        exit(1)

    if output_path.suffix.lower() != ".srctrldb":
        print(f"Error: Output file must have .srctrldb extension: {output_path}")
        exit(1)

    # Load JSON data
    with open(input_path, "r", encoding="utf-8") as f:
        data = json.load(f)

    symbols = data.get("symbols", [])
    references = data.get("references", [])
    modules = data.get("modules", [])

    # Open (and clear) the Sourcetrail DB
    db = SourcetrailDB.open(output_path, clear=True)

    # STEP 0: Gather all unique file paths and record them in the DB
    file_path_map = {}
    unique_files = set()

    # Collect files from symbols
    for sym in symbols:
        file_path = sym.get("File", "")
        if file_path:
            unique_files.add(file_path)

    # Collect files from references (if relevant)
    for ref in references:
        file_path = ref.get("File", "")
        if file_path:
            unique_files.add(file_path)

    # Record each file with language "go"
    for fpath in unique_files:
        abs_path = Path(fpath).resolve()
        file_id = db.record_file(abs_path)
        # If "go" is not recognized, you may have to use "cpp" for highlighting
        db.record_file_language(file_id, "go")
        file_path_map[fpath] = file_id

    # A map from our internal Symbol.ID to Numbat's recorded ID
    symbol_id_map = {}

    # We may want a helper for package nodes
    #   Key: (package path) -> numbat_id
    package_map = {}

    # STEP 1: Insert all symbols, and record their locations
    for sym in symbols:
        sym_id = sym["ID"]
        sym_name = sym["Name"]
        sym_kind = sym["Kind"]
        package_path = sym["PackagePath"]
        receiver_str = sym["Receiver"]  # for methods

        # Ensure we have a package node if needed
        pkg_id = None
        if package_path:
            # If not already recorded, create a "class" for the package:
            if package_path not in package_map:
                pkg_class_id = db.record_class(name=package_path, parent_id=None)
                package_map[package_path] = pkg_class_id
            pkg_id = package_map[package_path]

        # Decide how to record this symbol based on sym_kind
        if sym_kind == "package":
            # For a package symbol, just treat it as a class with the package path as name
            recorded_id = package_map.get(package_path)
            if recorded_id is None:
                # Record under package path or symbol name
                recorded_id = db.record_class(name=sym_name, parent_id=None)
                package_map[package_path] = recorded_id
        elif sym_kind == "type":
            # Record a new class. Parent is the package node if we have one.
            recorded_id = db.record_class(name=sym_name, parent_id=pkg_id)
        elif sym_kind == "field":
            # This is a struct field. If there's a receiver string, find that struct as parent.
            if receiver_str and receiver_str != "":
                raw_receiver = receiver_str.replace("(*", "").replace("*", "").replace(")", "")
                potential_type_name = raw_receiver.split(".")[-1]
                parent_id = None
                for s2 in symbols:
                    if (s2["Kind"] == "type" and
                        s2["Name"] == potential_type_name and
                        s2["PackagePath"] == package_path):
                        parent_id = symbol_id_map.get(s2["ID"])
                        break
                if parent_id is None:
                    parent_id = pkg_id
                recorded_id = db.record_field(name=sym_name, parent_id=parent_id)
            else:
                recorded_id = db.record_field(name=sym_name, parent_id=pkg_id)
        elif sym_kind in ("var", "const"):
            # Record a field. Parent is the package node
            recorded_id = db.record_field(name=sym_name, parent_id=pkg_id)
        elif sym_kind in ("func", "method"):
            # Method if there's a receiver (making it a method)
            if receiver_str and receiver_str != "":
                raw_receiver = receiver_str.replace("(*", "").replace("*", "").replace(")", "")
                potential_type_name = raw_receiver.split(".")[-1]
                parent_id = None
                for s2 in symbols:
                    if (s2["Kind"] == "type" and
                        s2["Name"] == potential_type_name and
                        s2["PackagePath"] == package_path):
                        parent_id = symbol_id_map.get(s2["ID"])
                        break
                if parent_id is None:
                    parent_id = pkg_id
                recorded_id = db.record_method(name=sym_name, parent_id=parent_id)
            else:
                # Standalone function
                recorded_id = db.record_method(name=sym_name, parent_id=pkg_id)
        else:
            # Fallback: record as field
            recorded_id = db.record_field(name=sym_name, parent_id=pkg_id)

        # Store the Numbat ID in our map
        symbol_id_map[sym_id] = recorded_id

        # Record symbol location if we have file info
        file_path = sym.get("File", "")
        line = sym.get("Line", 0)
        col = sym.get("Column", 0)
        if file_path and file_path in file_path_map and line > 0 and col > 0:
            file_id = file_path_map[file_path]
            # Approximate end column by adding length of the symbol name minus 1
            start_line = line
            start_col = col
            end_line = line
            end_col = col + len(sym_name) - 1 if len(sym_name) > 0 else col
            db.record_symbol_location(recorded_id, file_id, start_line, start_col, end_line, end_col)

    # STEP 2: Insert references
    for ref in references:
        from_id = ref["FromID"]
        to_id = ref["ToID"]
        ref_type = ref.get("RefType", "usage")
        from_numbat_id = symbol_id_map.get(from_id)
        to_numbat_id = symbol_id_map.get(to_id)
        if from_numbat_id is not None and to_numbat_id is not None:
            # Skip function->package usage references
            from_sym = next((s for s in symbols if s["ID"] == from_id), None)
            to_sym = next((s for s in symbols if s["ID"] == to_id), None)
            if (ref_type in ("usage", "call")
                and from_sym and to_sym
                and from_sym["Kind"] in ("func","method")
                and to_sym["Kind"] == "package"):
                continue

            if ref_type == "call":
                ref_id = db.record_ref_call(from_numbat_id, to_numbat_id)
            elif ref_type == "implements":
                ref_id = db.record_ref_inheritance(from_numbat_id, to_numbat_id)
            elif ref_type == "import":
                ref_id = db.record_ref_import(from_numbat_id, to_numbat_id)
            elif ref_type == "embeds":
                ref_id = db.record_ref_inheritance(from_numbat_id, to_numbat_id)
            else:
                ref_id = db.record_ref_usage(from_numbat_id, to_numbat_id)

            # Now record reference location if we have a file/line/column
            file_path = ref.get("File", "")
            line = ref.get("Line", 0)
            col = ref.get("Column", 0)
            if file_path in file_path_map and line > 0 and col > 0:
                ref_file_id = file_path_map[file_path]
                # Approximate highlight length by using the 'to_sym' name length if present
                usage_length = len(to_sym["Name"]) if to_sym and to_sym.get("Name") else 1
                end_col = col + max(1, usage_length) - 1
                db.record_reference_location(ref_id, ref_file_id, line, col, line, end_col)

    # Optionally, record each module as a top-level namespace node in the DB.
    for m in modules:
        mod_path = m.get("path", "")
        mod_ver = m.get("version", "")
        if mod_path:
            # Combining path & version for the node name
            full_name = mod_path
            if mod_ver:
                full_name += f"@{mod_ver}"
            db.record_namespace(name=full_name, parent_id=None)
    
    db.commit()
    db.close()

    print(f"✅ Successfully created Sourcetrail DB at: {output_path}")
    print(f"✅ Successfully created Sourcetrail DB at: {output_path}")

if __name__ == '__main__':
    main()