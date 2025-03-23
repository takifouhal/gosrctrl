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

def get_or_create_package_namespace(package_path, module_map, db, package_map):
    """
    Create or retrieve a nested namespace node for the given package_path
    under the appropriate module. If package_path matches or starts with a
    known module path, we nest under that module. Otherwise, we place it
    under an EXTERNAL namespace for third-party or unknown packages.
    """
    if package_path in package_map:
        return package_map[package_path]

    # Find the module that best matches this package path (longest prefix match)
    best_module = None
    best_len = 0
    for mp in module_map:
        # skip the "EXTERNAL" sentinel if present
        if mp == "EXTERNAL":
            continue
        if package_path == mp or package_path.startswith(mp + "/"):
            if len(mp) > best_len:
                best_len = len(mp)
                best_module = mp

    # If no suitable module found, place under EXTERNAL
    if best_module is None:
        best_module = "EXTERNAL"
        if "EXTERNAL" not in module_map:
            external_id = db.record_namespace(
                name="EXTERNAL",
                parent_id=None,
                is_indexed=False
            )
            module_map["EXTERNAL"] = external_id

        parent_id = module_map["EXTERNAL"]
        parts = package_path.split("/")
    else:
        parent_id = module_map[best_module]
        remainder = package_path[len(best_module):].lstrip("/")
        parts = remainder.split("/") if remainder else []

    current_parent = parent_id
    current_path = best_module

    # Create nested namespaces for each path segment
    for p in parts:
        new_path = current_path + "/" + p if current_path != "EXTERNAL" else p
        if new_path in package_map:
            current_parent = package_map[new_path]
            current_path = new_path
            continue
        else:
            ns_id = db.record_namespace(
                name=p,
                parent_id=current_parent,
                is_indexed=True,
                delimiter="/"
            )
            package_map[new_path] = ns_id
            current_parent = ns_id
            current_path = new_path

    package_map[package_path] = current_parent
    return current_parent

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

    # STEP 0: Build a top-level namespace for each module in the JSON
    # The first entry in 'modules' should be the main module; mark it as indexed.
    module_map = {}
    for idx, m in enumerate(modules):
        mod_path = m.get("path", "")
        mod_ver = m.get("version", "")
        if mod_path:
            # Combine path & version for the node name
            full_name = mod_path
            if mod_ver:
                full_name += f"@{mod_ver}"
            # The first module is presumably the main module, so is_indexed = True
            is_main = (idx == 0)
            mod_id = db.record_namespace(
                name=full_name,
                parent_id=None,
                is_indexed=is_main,
                delimiter="/"
            )
            module_map[mod_path] = mod_id

    # Prepare a special EXTERNAL node for anything not belonging to known modules
    # We'll create it on-demand inside get_or_create_package_namespace() if needed.

    # STEP 1: Gather unique file paths and record them with language "go"
    file_path_map = {}
    unique_files = set()

    # Collect files from symbols
    for sym in symbols:
        file_path = sym.get("File", "")
        if file_path:
            unique_files.add(file_path)

    # Collect files from references
    for ref in references:
        file_path = ref.get("File", "")
        if file_path:
            unique_files.add(file_path)

    for fpath in unique_files:
        abs_path = Path(fpath).resolve()
        file_id = db.record_file(abs_path)
        # For best results, set language to "go" if recognized by your Sourcetrail version
        db.record_file_language(file_id, "go")
        file_path_map[fpath] = file_id

    # A map from our Symbol.ID to the recorded Numbat symbol ID
    symbol_id_map = {}

    # We'll store package namespace IDs here
    package_map = {}

    # STEP 2: Insert all symbols. For "package" kind, we create a namespace node;
    # for other kinds, we record them under their package parent as before.
    for sym in symbols:
        sym_id = sym["ID"]
        sym_name = sym["Name"]
        sym_kind = sym["Kind"]
        package_path = sym.get("PackagePath", "")
        receiver_str = sym.get("Receiver", "")

        # We'll use the symbol's Sig for hover text if present
        hover_display = sym.get("Sig", "")

        # Identify (or create) the package namespace that owns this symbol
        pkg_parent_id = None
        if package_path:
            pkg_parent_id = get_or_create_package_namespace(package_path, module_map, db, package_map)

        # Decide how to record the symbol
        indexed = not sym.get("External", False)

        if sym_kind == "package":
            # Just map the package itself to the namespace
            # (We already created it above, so record that as the symbol ID.)
            recorded_id = pkg_parent_id
        elif sym_kind == "interface":
            recorded_id = db.record_class(
                name=sym_name,
                parent_id=pkg_parent_id,
                postfix="(interface)",
                is_indexed=indexed
            )
        elif sym_kind == "type":
            # A user-defined non-interface type
            recorded_id = db.record_class(
                name=sym_name,
                parent_id=pkg_parent_id,
                is_indexed=indexed
            )
        elif sym_kind == "field":
            if receiver_str:
                raw_receiver = receiver_str.replace("(*", "").replace("*", "").replace(")", "")
                potential_type_name = raw_receiver.split(".")[-1]
                parent_id = None
                for s2 in symbols:
                    if (s2["Kind"] == "type" and
                        s2["Name"] == potential_type_name and
                        s2.get("PackagePath", "") == package_path):
                        parent_id = symbol_id_map.get(s2["ID"])
                        break
                if parent_id is None:
                    parent_id = pkg_parent_id
                recorded_id = db.record_field(
                    name=sym_name,
                    parent_id=parent_id,
                    is_indexed=indexed
                )
            else:
                recorded_id = db.record_field(
                    name=sym_name,
                    parent_id=pkg_parent_id,
                    is_indexed=indexed
                )
        elif sym_kind in ("var", "const"):
            recorded_id = db.record_global_variable(
                name=sym_name,
                parent_id=pkg_parent_id,
                is_indexed=indexed
            )
        elif sym_kind in ("func", "method"):
            if receiver_str:
                raw_receiver = receiver_str.replace("(*", "").replace("*", "").replace(")", "")
                potential_type_name = raw_receiver.split(".")[-1]
                parent_id = None
                for s2 in symbols:
                    if (s2["Kind"] in ("type", "interface") and
                        s2["Name"] == potential_type_name and
                        s2.get("PackagePath", "") == package_path):
                        parent_id = symbol_id_map.get(s2["ID"])
                        break
                if parent_id is None:
                    parent_id = pkg_parent_id
                recorded_id = db.record_method(
                    name=sym_name,
                    parent_id=parent_id,
                    is_indexed=indexed
                )
            else:
                recorded_id = db.record_function(
                    name=sym_name,
                    parent_id=pkg_parent_id,
                    is_indexed=indexed
                )
        else:
            # Fallback: just record as a field
            recorded_id = db.record_field(
                name=sym_name,
                parent_id=pkg_parent_id,
                is_indexed=indexed
            )

        symbol_id_map[sym_id] = recorded_id

        # Record location if available
        file_path = sym.get("File", "")
        line = sym.get("Line", 0)
        col = sym.get("Column", 0)
        if file_path and file_path in file_path_map and line > 0 and col > 0:
            file_id = file_path_map[file_path]
            start_line = line
            start_col = col
            end_line = line
            end_col = col + max(1, len(sym_name)) - 1
            db.record_symbol_location(recorded_id, file_id, start_line, start_col, end_line, end_col)

    # STEP 3: Insert references (calls, usages, imports, etc.)
    for ref in references:
        from_id = ref["FromID"]
        to_id = ref["ToID"]
        ref_type = ref.get("RefType", "usage")
        from_numbat_id = symbol_id_map.get(from_id)
        to_numbat_id = symbol_id_map.get(to_id)
        if from_numbat_id is not None and to_numbat_id is not None:
            # Skip function->package usage references that may appear spurious
            from_sym = next((s for s in symbols if s["ID"] == from_id), None)
            to_sym = next((s for s in symbols if s["ID"] == to_id), None)
            if not (from_sym and to_sym):
                continue

            # Skip function->package usage references that may appear spurious
            if (ref_type in ("usage", "call")
                and from_sym["Kind"] in ("func", "method")
                and to_sym["Kind"] == "package"):
                continue

            # Skip trivial usage of var/const in the same package to reduce noise
            # (only if ref_type is "usage")
            if (ref_type == "usage"
                and from_sym.get("PackagePath") == to_sym.get("PackagePath")
                and to_sym["Kind"] in ("var", "const")):
                continue

            if ref_type == "call":
                ref_id = db.record_ref_call(from_numbat_id, to_numbat_id)
            elif ref_type in ("implements", "embeds"):
                # "implements" or "embeds" => use inheritance
                ref_id = db.record_ref_inheritance(from_numbat_id, to_numbat_id)
            elif ref_type == "import":
                ref_id = db.record_ref_import(from_numbat_id, to_numbat_id)
            elif ref_type == "write":
                # Numbat doesn't have a separate "write" edge, so record as usage
                # Optionally add special hover text or logging if needed
                ref_id = db.record_ref_usage(from_numbat_id, to_numbat_id)
            else:
                # default usage
                ref_id = db.record_ref_usage(from_numbat_id, to_numbat_id)

            file_path = ref.get("File", "")
            line = ref.get("Line", 0)
            col = ref.get("Column", 0)
            if file_path in file_path_map and line > 0 and col > 0:
                ref_file_id = file_path_map[file_path]
                usage_length = len(to_sym["Name"]) if to_sym and to_sym.get("Name") else 1
                end_col = col + max(1, usage_length) - 1
                db.record_reference_location(ref_id, ref_file_id, line, col, line, end_col)

    db.commit()
    db.close()

    print(f"✅ Successfully created Sourcetrail DB at: {output_path}")
    print(f"✅ Successfully created Sourcetrail DB at: {output_path}")

if __name__ == '__main__':
    main()