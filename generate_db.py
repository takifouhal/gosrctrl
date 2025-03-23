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
import sys

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
    
    # Debug information about paths
    print(f"DEBUG: Using input file: {input_path.absolute()}", file=sys.stderr)
    print(f"DEBUG: Target output file: {output_path.absolute()}", file=sys.stderr)
    print(f"DEBUG: Current working directory: {os.getcwd()}", file=sys.stderr)
    
    # Check if output directory exists and is writable
    output_dir = output_path.parent
    if not output_dir.exists():
        print(f"WARNING: Output directory does not exist: {output_dir}", file=sys.stderr)
        try:
            output_dir.mkdir(parents=True, exist_ok=True)
            print(f"DEBUG: Created output directory: {output_dir}", file=sys.stderr)
        except Exception as e:
            print(f"ERROR: Failed to create output directory: {e}", file=sys.stderr)
            exit(1)
    
    if not os.access(output_dir, os.W_OK):
        print(f"ERROR: Output directory is not writable: {output_dir}", file=sys.stderr)
        exit(1)

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
    try:
        print(f"DEBUG: Opening Sourcetrail DB at: {output_path.absolute()}", file=sys.stderr)
        db = SourcetrailDB.open(output_path, clear=True)
        print(f"DEBUG: Successfully opened Sourcetrail DB", file=sys.stderr)
    except Exception as e:
        print(f"ERROR: Failed to open Sourcetrail DB: {e}", file=sys.stderr)
        exit(1)

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

    # STEP 2: Insert all symbols.
    for sym in symbols:
        sym_id = sym["ID"]
        sym_name = sym["Name"]
        sym_kind = sym["Kind"]
        package_path = sym.get("PackagePath", "")
        receiver_str = sym.get("Receiver", "")
        hover_display = sym.get("Sig", "")
        indexed = not sym.get("External", False)

        # Identify (or create) the package namespace that owns this symbol
        pkg_parent_id = None
        if package_path:
            pkg_parent_id = get_or_create_package_namespace(package_path, module_map, db, package_map)

        # If the parser assigned a ParentID, we try that first.
        parent_id = pkg_parent_id
        stored_parent_id = sym.get("ParentID", 0)
        if stored_parent_id != 0:
            mapped_parent_id = symbol_id_map.get(stored_parent_id)
            if mapped_parent_id is not None:
                parent_id = mapped_parent_id

        # Decide how to record the symbol based on sym_kind
        if sym_kind == "package":
            # The package itself maps to the namespace
            recorded_id = pkg_parent_id

        elif sym_kind == "struct":
            recorded_id = db.record_struct(
                name=sym_name,
                parent_id=parent_id,
                
                is_indexed=indexed
            )

        elif sym_kind == "interface":
            recorded_id = db.record_interface(
                name=sym_name,
                parent_id=parent_id,
                
                is_indexed=indexed
            )

        elif sym_kind == "field":
            # First, check if we have a valid ParentID from the parser
            if stored_parent_id != 0:
                mapped_parent_id = symbol_id_map.get(stored_parent_id)
                if mapped_parent_id is not None:
                    parent_id = mapped_parent_id
                else:
                    parent_id = pkg_parent_id
            elif receiver_str:
                # If no explicit ParentID, attempt to locate parent type via receiver
                raw_receiver = receiver_str.replace("(*", "").replace("*", "").replace(")", "")
                potential_type_name = raw_receiver.split(".")[-1]
                parent_found = None
                for s2 in symbols:
                    if (
                        s2["Kind"] in ("type", "struct", "interface")
                        and s2["Name"] == potential_type_name
                        and s2.get("PackagePath", "") == package_path
                    ):
                        candidate_numbat_id = symbol_id_map.get(s2["ID"])
                        if candidate_numbat_id is not None:
                            parent_found = candidate_numbat_id
                            break
                if parent_found is None:
                    parent_found = pkg_parent_id
                parent_id = parent_found
            else:
                # Fallback to the package-level parent
                parent_id = pkg_parent_id

            recorded_id = db.record_field(
                name=sym_name,
                parent_id=parent_id,
                
                is_indexed=indexed
            )

        elif sym_kind == "func":
            recorded_id = db.record_function(
                name=sym_name,
                parent_id=parent_id,
                
                is_indexed=indexed
            )

        elif sym_kind == "method":
            # For methods, we have a receiver string. Locate the parent struct/interface if possible.
            raw_receiver = receiver_str.replace("(*", "").replace("*", "").replace(")", "")
            potential_type_name = raw_receiver.split(".")[-1]
            parent_found = None
            for s2 in symbols:
                if (
                    s2["Kind"] in ("type", "struct", "interface")
                    and s2["Name"] == potential_type_name
                    and s2.get("PackagePath", "") == package_path
                ):
                    candidate_numbat_id = symbol_id_map.get(s2["ID"])
                    if candidate_numbat_id is not None:
                        parent_found = candidate_numbat_id
                        break
            if parent_found is None:
                parent_found = pkg_parent_id

            recorded_id = db.record_method(
                name=sym_name,
                parent_id=parent_found,
                
                is_indexed=indexed
            )

        elif sym_kind in ("var", "const"):
            # For top-level vars/consts, represent as a GLOBAL_VARIABLE
            recorded_id = db.record_global_variable(
                name=sym_name,
                parent_id=parent_id,
                
                is_indexed=indexed
            )

        else:
            # Fallback: treat anything else as a field or generic type usage
            recorded_id = db.record_field(
                name=sym_name,
                parent_id=parent_id,
                
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
    print(f"DEBUG: Processing {len(references)} references", file=sys.stderr)
    type_relation_count = sum(1 for ref in references if ref.get("RefType") in ("implements", "embeds"))
    print(f"DEBUG: Found {type_relation_count} interface-implementation and embedding relationships", file=sys.stderr)
    
    for ref in references:
        from_id = ref["FromID"]
        to_id = ref["ToID"]
        ref_type = ref.get("RefType", "usage")
        from_numbat_id = symbol_id_map.get(from_id)
        to_numbat_id = symbol_id_map.get(to_id)
        if from_numbat_id is not None and to_numbat_id is not None:
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

            # Now record the reference
            if ref_type == "call":
                ref_id = db.record_ref_call(from_numbat_id, to_numbat_id)
            elif ref_type in ("implements", "embeds"):
                # Log details about inheritance relationships
                from_sym_name = from_sym.get("Name", "unknown") if from_sym else "unknown"
                to_sym_name = to_sym.get("Name", "unknown") if to_sym else "unknown"
                from_kind = from_sym.get("Kind", "unknown") if from_sym else "unknown"
                to_kind = to_sym.get("Kind", "unknown") if to_sym else "unknown"
                
                if ref_type == "implements":
                    # In Go, 'implements' means a concrete type implements an interface
                    # We check that the relationship makes semantic sense
                    if to_kind != "interface" and to_kind != "unknown":
                        print(f"WARNING: Unusual implements relationship: {from_kind} '{from_sym_name}' → {to_kind} '{to_sym_name}' (target should be interface)", file=sys.stderr)
                        
                    print(f"DEBUG: Recording implementation relationship: {from_kind} '{from_sym_name}' → {to_kind} '{to_sym_name}'", file=sys.stderr)
                else:  # embeds
                    # For embedding, we want to show composition relationships
                    # This can be struct→struct or interface→interface
                    if from_kind == "struct" and to_kind != "struct" and to_kind != "unknown":
                        print(f"WARNING: Unusual struct embedding: {from_kind} '{from_sym_name}' embeds {to_kind} '{to_sym_name}' (not a struct)", file=sys.stderr)
                    
                    if from_kind == "interface" and to_kind != "interface" and to_kind != "unknown":
                        print(f"WARNING: Unusual interface embedding: {from_kind} '{from_sym_name}' embeds {to_kind} '{to_sym_name}' (not an interface)", file=sys.stderr)
                        
                    print(f"DEBUG: Recording embedding relationship: {from_kind} '{from_sym_name}' → {to_kind} '{to_sym_name}'", file=sys.stderr)
                
                # Both 'implements' and 'embeds' are mapped to inheritance in Sourcetrail
                ref_id = db.record_ref_inheritance(from_numbat_id, to_numbat_id)
            elif ref_type == "import":
                ref_id = db.record_ref_import(from_numbat_id, to_numbat_id)
            elif ref_type == "write":
                # NOTE: Numbat does not have a dedicated 'write' reference type,
                # so we map "write" to 'usage' here.
                ref_id = db.record_ref_usage(from_numbat_id, to_numbat_id)
            else:
                ref_id = db.record_ref_usage(from_numbat_id, to_numbat_id)

            # Record reference location if available
            file_path = ref.get("File", "")
            line = ref.get("Line", 0)
            col = ref.get("Column", 0)
            if file_path in file_path_map and line > 0 and col > 0:
                ref_file_id = file_path_map[file_path]
                usage_length = len(to_sym["Name"]) if to_sym and to_sym.get("Name") else 1
                end_col = col + max(1, usage_length) - 1
                db.record_reference_location(ref_id, ref_file_id, line, col, line, end_col)

    try:
        print(f"DEBUG: Committing Sourcetrail DB changes", file=sys.stderr)
        db.commit()
        print(f"DEBUG: Successfully committed changes", file=sys.stderr)
        db.close()
        print(f"DEBUG: Closed Sourcetrail DB", file=sys.stderr)
        
        # Verify the file exists after creation
        if output_path.exists():
            print(f"DEBUG: Verified file exists: {output_path.absolute()}, size: {output_path.stat().st_size} bytes", file=sys.stderr)
        else:
            print(f"ERROR: File does not exist after creation: {output_path.absolute()}", file=sys.stderr)
            
        # Check if project file was created too
        project_path = output_path.with_suffix(".srctrlprj")
        if project_path.exists():
            print(f"DEBUG: Project file exists: {project_path.absolute()}, size: {project_path.stat().st_size} bytes", file=sys.stderr)
        else:
            print(f"DEBUG: Project file was not created: {project_path.absolute()}", file=sys.stderr)
    except Exception as e:
        print(f"ERROR during final steps: {e}", file=sys.stderr)
        exit(1)

    print(f"✅ Successfully created Sourcetrail DB at: {output_path}")

if __name__ == "__main__":
    main()