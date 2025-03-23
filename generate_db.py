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

    # Open (and clear) the Sourcetrail DB
    db = SourcetrailDB.open(output_path, clear=True)

    # A map from our internal Symbol.ID to Numbat's recorded ID
    symbol_id_map = {}

    # We may want a helper for package nodes
    #   Key: (package path) -> numbat_id
    package_map = {}

    # Step 1: Insert all symbols
    for sym in symbols:
        sym_id = sym["ID"]
        sym_name = sym["Name"]
        sym_kind = sym["Kind"]
        package_path = sym["PackagePath"]
        receiver_str = sym["Receiver"]  # for methods

        # Ensure we have a package node if needed
        # We'll treat package as a "class" as well, just to have a parent container.
        # You can also store packages in a separate dictionary if you want them as top-level.
        pkg_id = None
        if package_path:
            # If not already recorded, create a "class" for the package:
            if package_path not in package_map:
                pkg_class_id = db.record_class(name=package_path, parent_id=None)
                package_map[package_path] = pkg_class_id
            pkg_id = package_map[package_path]

        # Decide how to record this symbol based on sym_kind
        # (We do not distinguish interface vs struct right now, both are "class")
        if sym_kind == "package":
            # For a package symbol, just treat it as a class with the package path as name
            # or symbol name if it differs
            # Some packages might have multiple synonyms (like _test). Use package_path.
            recorded_id = package_map.get(package_path)
            if recorded_id is None:
                # Record it under the same name as package_path or sym_name
                recorded_id = db.record_class(name=sym_name, parent_id=None)
                package_map[package_path] = recorded_id
        elif sym_kind == "type":
            # Record a new class. Parent is the package node if we have one.
            recorded_id = db.record_class(name=sym_name, parent_id=pkg_id)
        elif sym_kind in ("var", "const"):
            # Record a field. Parent is the package node
            recorded_id = db.record_field(name=sym_name, parent_id=pkg_id)
        elif sym_kind in ("func", "method"):
            # Check if we have a receiver -> that means it might be a method of some type
            if receiver_str and receiver_str != "":
                # Try to parse the type name from receiver_str
                # e.g., "(*github.com/foo/bar.MyStruct)" => "github.com/foo/bar.MyStruct"
                # We'll do a simple approach by removing pointers and parens
                # Then see if we can find the type in the symbol list or not
                raw_receiver = receiver_str
                raw_receiver = raw_receiver.replace("(*", "").replace("*", "").replace(")", "")
                # raw_receiver might be "github.com/foo/bar.MyStruct" or "MyStruct"
                # We look in the package_map to see if it matches a known pkg, or we just find last
                # portion as the type name
                # A simpler approach is to see if there's a '.' in raw_receiver
                # We'll do naive: "pkgName.Type"
                # We'll see if the type was recorded as a class name in that package
                # but for this example, let's do a simple approach:
                parent_symbol_id = None
                # We'll guess the type name is the last segment after a dot
                # e.g. "github.com/foo/bar.MyStruct" => "MyStruct"
                # Then we look for that class in the same package
                potential_type_name = raw_receiver.split(".")[-1]
                # There's no guarantee that we can find it easily. For demonstration:
                # We'll search for a symbol with kind="type" and Name=potential_type_name in the same package
                # and get that class's Numbat ID. This is simplistic.
                parent_id = None
                for s2 in symbols:
                    if s2["Kind"] == "type" and s2["Name"] == potential_type_name and s2["PackagePath"] == package_path:
                        parent_id = symbol_id_map.get(s2["ID"])
                        break

                # If we didn't find it, fallback to package
                if parent_id is None:
                    parent_id = pkg_id

                # record the method
                recorded_id = db.record_method(name=sym_name, parent_id=parent_id)
            else:
                # Standalone function
                recorded_id = db.record_method(name=sym_name, parent_id=pkg_id)
        else:
            # Fallback: record as field or method?
            # We'll treat unknown kinds as fields
            recorded_id = db.record_field(name=sym_name, parent_id=pkg_id)

        symbol_id_map[sym_id] = recorded_id

    # Step 2: Insert references
    for ref in references:
        from_id = ref["FromID"]
        to_id = ref["ToID"]
        # Look up the recorded IDs in Numbat
        from_numbat_id = symbol_id_map.get(from_id)
        to_numbat_id = symbol_id_map.get(to_id)
        if from_numbat_id is not None and to_numbat_id is not None:
            db.record_ref_usage(from_numbat_id, to_numbat_id)

    db.commit()
    db.close()

    print(f"✅ Successfully created Sourcetrail DB at: {output_path}")

if __name__ == '__main__':
    main()