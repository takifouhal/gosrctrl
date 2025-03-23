# Numbat Integration for GoSrcCtrl Developer Guide

## Overview  
GoSrcCtrl uses **Numbat**, a Python API for creating and manipulating Sourcetrail databases ([GitHub - quarkslab/numbat: Library to manipulate and create Sourcetrail databases](https://github.com/quarkslab/numbat#:~:text=Image)), to convert Go source code into a navigable Sourcetrail project. This guide covers how to use Numbat’s Python API in the `gosrctrl` context to improve the Sourcetrail database hierarchy and usability. We’ll focus on five key areas aligned with the enhancement tasks for GoSrcCtrl: organizing namespaces (Go modules/packages), recording various symbols (types, functions, constants, etc.), enriching nodes with hover information, categorizing references (calls, usages, implements, etc.), and handling external or standard library symbols with minimal noise. The goal is to help developers working on `generate_db.py` call the right Numbat APIs to effectively represent Go code structure.

## Getting Started  
First, set up a Sourcetrail database using Numbat’s `SourcetrailDB` class. Open or create a database (using `open(..., clear=True)` to start fresh) and remember to **commit** and **close** when done ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=from%20numbat%20import%20SourcetrailDB%20from,pathlib%20import%20Path)) ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=db%20%3D%20SourcetrailDB.open%28Path%28%27my_database%27%29%2C%20clear%3DTrue%29%20,close)). For example: 

```python
from numbat import SourcetrailDB
from pathlib import Path

# Create or open a Sourcetrail DB (clearing if it exists)
db = SourcetrailDB.open(Path(output_path), clear=True)

# ... record symbols and references ...

db.commit()
db.close()
``` 

This produces a `.srctrldb` database (and a `.srctrlprj` project file) that you can open in Sourcetrail ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=db)). When building the DB, ensure all relevant symbols and references are recorded **before** calling `commit()`. 

**Tip:** To get syntax highlighting and source navigation in Sourcetrail, record source files and their language. For example, use `db.record_file(file_path)` and `db.record_file_language(file_id, "go")` to register each Go source file, and use `db.record_symbol_location` to map symbols to their locations in code (line/column). This allows Sourcetrail to show definitions and references inline in the source viewer.

## Namespaces and Hierarchy (Go Modules & Packages)  
To reflect Go’s module and package hierarchy in Sourcetrail, use Numbat’s namespace-like symbols. Go modules and packages are not code symbols themselves, but we can model them as **module** and **package** nodes for organizational clarity:

- **Module:** Represent the Go module (from `go.mod`) as a top-level namespace. Use `record_module(name=..., delimiter="/")` to create a MODULE symbol ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=Record%20a%20MODULE%20symbol%20into,the%20DB)). The `name` can be the module path (e.g. `"github.com/user/project"`). Setting `delimiter="/"` will use “/” in fully-qualified names (appropriate for Go import paths). This returns an `id` for the module node.  
  *Example:* `mod_id = db.record_module(name="github.com/example/myapp", delimiter="/")`

- **Package:** For each Go package, create a PACKAGE (or NAMESPACE) symbol. Use `record_package(name=..., parent_id=...)` to nest the package under its module ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_package)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). If your module contains sub-packages (e.g. `module/subpkg`), you can create a hierarchy by calling `record_package` for each segment or using parent relationships. Typically, a Go package’s import path can be split by “/” – you might represent the top-level package as a direct child of the module, and deeper components as nested packages. All `record_package` and `record_namespace` calls share a similar signature (name, prefix, postfix, etc.) and can use `parent_id` to nest hierarchically.  
  *Example:*  
  ```python
  pkg_id = db.record_package(name="mypkg", parent_id=mod_id)
  subpkg_id = db.record_package(name="utils", parent_id=pkg_id)
  ```  
  This would reflect a package path `mypkg/utils` under the module. By default, Numbat uses a C++-style delimiter `::`, but since the parent’s delimiter is used for children ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=The%20delimiter%20of%20the%20element%2C,parent%20delimiter%20will%20be%20used)), setting the module’s delimiter to `"/"` ensures the whole hierarchy uses slash separators.

**Gotcha:** If you do not assign a parent for package nodes, they will appear as separate top-level namespaces. Always use the module’s `id` as parent for packages to keep them grouped. If your project doesn’t use modules, you can still use a single top-level namespace (e.g. project name) as the parent for all packages. Use `is_indexed=True` (default) for real packages. The `hover_display` field can be set to show extra info (for example, the module version or full import path) when hovering over the node ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)), but it’s optional.

## Recording Symbols (Types, Functions, Variables, etc.)  
Once namespaces (module/packages) are in place, record the actual Go symbols within their respective package node. Numbat provides various `record_*` functions to create symbols in the DB, each corresponding to a kind of code element. All symbol-recording functions share a common signature ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=record_XX%28%20name%3D,parent_id%3DNone%2C%20is_indexed%3DTrue%2C)): you provide at least a `name` and optionally `prefix`, `postfix`, `parent_id`, `is_indexed`, and `hover_display`. The `parent_id` should typically be the enclosing namespace or type (to reflect scope), and `is_indexed` should be `True` for symbols that exist in your code (use `False` for “stub” symbols that have no definition in your code). The `hover_display` can enrich the symbol’s node with details like type signature. Below we cover common symbol types and the appropriate Numbat API calls:

### Types (Structs, Interfaces, Aliases)  
Go’s user-defined types (declared with `type`) can represent structs, interfaces, or other definitions. Use the Numbat function that best matches the kind of type: 
- **Struct Types:** Use `record_struct(name, parent_id=pkg_id, ...)` to record a struct type ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_struct)). This creates a STRUCT symbol in Sourcetrail (often with a specific icon). The parent should be the package’s node ID (so the struct appears under its package namespace).  
- **Interface Types:** Use `record_interface(name, parent_id=pkg_id, ...)` for interfaces ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=,record_enum)). It works similarly, creating an INTERFACE symbol.  
- **General Types or Aliases:** If a type doesn’t fall into a specific category, `record_class` is a generic type symbol creator (originally meant for classes) ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl · GitHub](https://github.com/takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=pkg_class_id%20%3D%20db)) ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl · GitHub](https://github.com/takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=,node%20if%20we%20have%20one)). In Go, you might use `record_class` for type aliases or other defined types that are not structs or interfaces. However, prefer `record_struct` or `record_interface` when those apply for clarity.  

For each type symbol, you can use `hover_display` to show additional context. For example, for a struct you might set `hover_display="type MyStruct struct {...}"` to show its definition snippet, or for an interface list its key methods. Example usage: 

```python
# Inside package 'mypkg'
struct_id = db.record_struct(name="Person", parent_id=pkg_id,
                             hover_display="type Person struct { Name string; Age int }")
iface_id = db.record_interface(name="Stringer", parent_id=pkg_id,
                               hover_display="type Stringer interface { String() string }")
alias_id = db.record_class(name="AgeType", parent_id=pkg_id,
                           hover_display="type AgeType = int")
```

**Note:** `prefix` and `postfix` can be used to decorate the name in the UI if desired. For instance, `prefix="struct "` or `postfix=" {}"` could be added to make it explicit, but this is optional since Sourcetrail’s icons and hover text usually suffice. All these functions return an integer ID for the new symbol (or `None` if insertion failed) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=Type%20Description%20%60int%20)), which you will use to record relationships and as parents for child symbols.

### Functions and Methods  
Go functions and methods are recorded with `record_function` or `record_method`. The difference is mainly semantic: 
- **Free Functions:** Use `record_function(name, parent_id=pkg_id, ...)` for package-level functions ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_function%28%20name%3D,)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=Record%20a%20FUNCTION%20symbol%20into,the%20DB)). The parent should be the package symbol (so the function is scoped to its package). This creates a FUNCTION symbol in the DB.  
- **Methods (associated with a receiver type):** Use `record_method(name, parent_id=type_id, ...)` for methods defined on a receiver (struct or any named type) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_method)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). Set `parent_id` to the ID of the type that owns the method (the receiver type). This creates a METHOD symbol under that type’s scope. In Sourcetrail, it will appear nested under the type.  

For each function or method, consider adding signature info as hover text. For example, `hover_display="func DoThing(x int, y string) bool"` will show the full signature when the user hovers over the function node. You might also use `postfix="()"` to visually distinguish it as a function in the graph view (e.g., symbol name appears as `DoThing()` instead of just `DoThing`). Example:

```python
# A standalone function in the package
fn_id = db.record_function(name="PrintHello", parent_id=pkg_id,
                           hover_display="func PrintHello(name string)")
# A method on Person struct
meth_id = db.record_method(name="Greet", parent_id=struct_id,
                           hover_display="func (p *Person) Greet() string")
```

**Developer Notes:** Ensure you choose `record_function` vs `record_method` correctly. In earlier implementations, you might have used `record_method` for both, but using `record_function` for free functions is clearer. The `parent_id` for a free function can be a package ID (as above) or `None` if you want it truly global, but in Go it’s better to attach it to the package namespace for organization. The Numbat API doesn’t enforce a difference beyond naming, but it may affect how icons are shown or how future queries categorize the symbol.

### Variables, Constants, and Fields  
Go constants and package-level variables should be recorded as **global variables** in Sourcetrail, while struct fields are **fields** of a struct type:

- **Package-Level Variables and Constants:** Use `record_global_variable(name, parent_id=pkg_id, ...)` ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_global_variable)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=Record%20a%20GLOBAL_VARIABLE%20symbol%20into,the%20DB)) for each global var or constant. This will create a GLOBAL_VARIABLE symbol (for example, a constant might still be represented as a global variable symbol in Sourcetrail). By providing the `parent_id` as the package, these symbols will appear under their package namespace. You might use `hover_display` to show the value or type (e.g., `hover_display="const Pi float64 = 3.14"` or `"var ConfigPath string"`). If you want to differentiate constants vs variables, you could set a prefix like `"const "` or `"var "`, but again, that’s optional.  
- **Struct Fields:** Use `record_field(name, parent_id=type_id, ...)` ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_field)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=Record%20a%20FIELD%20symbol%20into,the%20DB)) for fields of structs (or any member variables of a composite type). The parent should be the struct’s ID so that Sourcetrail knows the field belongs to that struct. For example, for `type Person struct { Name string }`, when recording the `Name` field, use `parent_id=struct_id` of `Person`. You typically don’t give fields a prefix/postfix (the default icon and context imply it), but you might set `hover_display` to show the field’s type (e.g., `"Name string"`).  

Example:  
```python
# Global constants/vars in package
const_id = db.record_global_variable(name="Pi", parent_id=pkg_id,
                                     hover_display="const Pi = 3.1415")
var_id = db.record_global_variable(name="DefaultName", parent_id=pkg_id,
                                   hover_display="var DefaultName = \"Anonymous\"")
# Field in Person struct
field_id = db.record_field(name="Name", parent_id=struct_id,
                            hover_display="Name string")
```

**Gotchas:** In the initial version, `gosrctrl` used `record_field` even for top-level vars and consts (with the package as parent). While this works, it categorizes them as fields (which is intended for members of a type). It’s better to use `record_global_variable` for clarity – these will be indexed as standalone variables and could have different icons or behaviors. Always attach fields to their type and globals to their package. If a field’s parent type is not found (e.g., referencing an external struct’s field), you may attach it to the package or mark it as non-indexed if it’s just a reference (see External Symbols section).

## Using Hover Text and Display Options  
Numbat allows adding extra context to symbols through optional parameters:
- **hover_display (str):** A string that will be shown when hovering over the symbol or reference in Sourcetrail ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). This is very useful for providing signatures, types, or other info without cluttering the node name. For example, for a function node `Add`, you might set `hover_display="func Add(a int, b int) int"`. For a variable, `hover_display="type: int"` or showing its value if a constant. Hover text does not affect the graph structure; it’s purely for user information.
- **prefix/postfix (str):** These let you add text before or after the symbol name in the displayed label ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=,show%20as%20a%20grey%20shadow)). They are optional and mainly cosmetic. In many cases, the symbol’s category icon and hover text make this unnecessary. However, you could use them to mimic Go syntax at a glance. For instance, using `prefix="func "` and `postfix="()"` for functions will display as `func Name()` in the graph. Or prefix interface names with `interface ` for clarity. Use these consistently if at all, and avoid overloading the name with too much detail. Keep the main `name` concise (e.g., just `Name` rather than `Name string` which is better in hover text).

**Example of enriching a symbol:**  
```python
# Record a function with signature in hover text
db.record_function(name="Compute", parent_id=pkg_id,
                   hover_display="func Compute(x float64) (y int, err error)")
```  
When viewed in Sourcetrail, the node labeled “Compute” will show the full signature on hover. This provides quick context without needing to jump to source. Use hover displays for anything that would help a developer understand a symbol at a glance: function parameters/returns, constant values, struct field types, interface method list, etc.

Similarly, references (edges) also accept an optional `hover_display` in many `record_ref_*` calls ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=required%20)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). For example, a call reference could carry a hover text like `"line 10: PrintHello()"` or other custom message. In practice, you might not need to set hover text on references if the source location is recorded (Sourcetrail will show the code snippet), but the option exists for clarity.

## Recording References (Calls, Usages, Imports, etc.)  
After all symbols are recorded, you should add relationships between them so Sourcetrail can show how everything is connected. Numbat’s `record_ref_*` functions create directed edges of various types in the database. Using the correct reference type for each relationship will improve the accuracy of the visualization (e.g., differentiating function calls from variable usages). Here are the common reference types relevant to Go and how to use them:

- **Function Calls:** Use `db.record_ref_call(caller_id, callee_id)` to record a CALL reference ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_ref_call)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). Here, `caller_id` is the symbol ID of the function or method making the call, and `callee_id` is the ID of the function/method being called. In Go, every function invocation should be a call edge. For example, if `PrintHello` calls `fmt.Println`, you’d record a call from the `PrintHello` function’s ID to the `Println` function’s ID (whether `Println` is a user-defined or external stub symbol). Use CALL for any invocation so that Sourcetrail can show it with the appropriate call arrow icon and allow "Go to callee/caller" navigation. 

- **Variable or Field Usage:** Use `db.record_ref_usage(user_id, target_id)` for general “uses” relationships ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_ref_usage)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). This is a catch-all for referencing a symbol in a non-call context. For instance, reading or writing a variable, accessing a constant, or using a field of a struct (if not specifically modeling it as a call). In the example from the Numbat tutorial, a method using a field was added as a USAGE edge ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=created%20with%20the%20commands%20,relation)) ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=class_id%20%3D%20db.record_class%28prefix%3D,record_ref_usage%28meth_id%2C%20field_id)). In Go, if a function `Compute` uses a package-level variable `DefaultName`, record `record_ref_usage(compute_id, defaultName_id)`. If a method accesses a struct’s field, record usage from the method to the field symbol. Essentially, **usage** is a dependency where one code entity references another without calling it.  

- **Type Reference (Implements/Embeds):** Go does not have classical inheritance, but you have interfaces and struct embedding. We can represent these relationships so that Sourcetrail shows a proper connection:
  - **Implements Interface:** If a struct type implements an interface (i.e., implements all its methods), you can use `record_ref_inheritance(struct_id, interface_id)` ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl · GitHub](https://github.com/takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=elif%20ref_type%20%3D%3D%20)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_ref_inheritance)). In Sourcetrail, INHERITANCE edges typically mean an “is-a” relationship. Using inheritance for interface implementation will show that the struct “inherits from” (implements) the interface, which effectively conveys the relationship. (There is no dedicated “implements” edge type in Numbat, so inheritance is a suitable choice as used in gosrctrl’s logic.)  
  - **Embedded Fields (Composition):** If struct A embeds struct B (`type A struct { B }`), it means A includes B’s fields and methods (a form of inheritance in behavior). This can also be modeled with `record_ref_inheritance(A_id, B_id)` to indicate A inherits from B ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl · GitHub](https://github.com/takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=ref_id%20%3D%20db)). This way, in the graph, A will point to B with an inheritance arrow, suggesting an embedded relationship.  
  - **Type Aliases or Usage:** If needed, `record_ref_type_usage(source_id, type_id)` can denote that one symbol uses a type. For example, a variable of custom struct type could be linked to the struct via a type usage edge. This is optional – if your project wants explicit visualization of type dependencies. (Numbat provides `record_ref_type_usage`, which adds a TYPE_USAGE edge, but if not critical, you can omit to reduce noise.)

- **Imports (Package Dependencies):** Use `db.record_ref_import(from_id, to_id)` for import relationships ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_ref_import)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). Typically, `from_id` would be the symbol representing the importing package (e.g., your package), and `to_id` would be the symbol for the imported package. If you recorded each Go package in your project as a symbol (and possibly created symbols for external packages as stubs), you should connect them with import edges. For example, if package `mypkg` imports the Go standard `fmt` package, ensure you have a symbol for `fmt` (perhaps a PACKAGE node marked non-indexed) and do `record_ref_import(mypkg_id, fmt_pkg_id)`. Sourcetrail will then show an "imports" relationship. This is especially useful for visualizing module dependencies. The `hover_display` on an import edge could optionally include alias or import details if needed, but usually just the edge is enough.

- **Other Reference Types:** Numbat also has `record_ref_member`, `record_ref_override`, etc., but these are more relevant to object-oriented languages (C++/Java). In Go’s context, you likely won’t use `record_ref_override` (no subclass method overrides) or `record_ref_member` explicitly (since membership is implied by parent_id for fields). **Calls, usage, inheritance, and import** cover most needs. If you encounter macros or annotations (unlikely in pure Go code), Numbat has edges for those too (e.g., `record_ref_macro_usage`), but Go doesn’t have an equivalent concept.

**Choosing the Right Reference:** The `RefType` from GoSrcCtrl’s JSON should guide which `record_ref_*` to use. For example, gosrctrl might label a reference as `"call"` or `"usage"` or `"implements"`, etc. Map those to the appropriate Numbat call as described above. Use CALL for anything that is a function invocation (even if it’s a method call or built-in function). Use USAGE for reading/writing variables, constants, or fields. Use INHERITANCE for implements or embedded struct relations. Use IMPORT for imports. If a reference in the JSON is ambiguous, err on the side of marking it as `record_ref_usage` by default – you can refine if needed. For instance, if a struct type is used as a field type in another struct, you might either ignore (since the field already connects them logically) or use a type usage edge if you want that link visible.

**Recording Reference Locations:** After adding a reference, you can (optionally) record the source location of that reference with `db.record_reference_location(ref_id, file_id, start_line, start_col, end_line, end_col)`. This highlights where the reference occurs in code. In gosrctrl, after calling `record_ref_*`, if you have the file and line info, use it to improve the user’s ability to jump to that usage in Sourcetrail ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl · GitHub](https://github.com/takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=if%20file_path%20in%20file_path_map%20and,0)) ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl · GitHub](https://github.com/takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=ref_file_id%20%3D%20file_path_map)). For example, for a call, highlight the function name being called.

## Optional Enhancements: Handling External or Unresolved Symbols  
Go programs often use symbols from the standard library or third-party packages. These symbols won’t be present in your code’s symbol list, but you may still want them represented in the Sourcetrail DB for navigability. There are a couple of strategies to handle external symbols while **minimizing noise**:

- **Create Stub Symbols (Non-Indexed):** You can create a minimal representation of external packages or functions as “stub” nodes that serve as targets for references. Mark them with `is_indexed=False` to indicate they weren’t actually defined in the indexed code ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=)). For example, if your code calls `fmt.Println`, you might do:  
  ```python
  fmt_pkg_id = db.record_package(name="fmt", parent_id=None, is_indexed=False)
  println_id = db.record_function(name="Println", parent_id=fmt_pkg_id, is_indexed=False,
                                  hover_display="func Println(a ...interface{})") 
  db.record_ref_call(caller_id, println_id)
  ```  
  This will introduce a `fmt` package node (likely greyed out or dim in Sourcetrail to show it’s external) and a `Println` function under it, also marked as external. The call from your function to this stub will now appear, and if a user clicks it, they’ll see the stub with whatever signature/info you provided. This preserves navigability (the user can follow the reference) without pulling in the entire external library. The downside is you need to create stubs for each external symbol you want to visualize. It’s wise to limit stub creation to important external functions or types to avoid clutter. Perhaps create stub package nodes for each imported external package and stub only the symbols actually referenced.

- **Unsolved Symbol References:** Alternatively, Numbat provides a way to record a reference to something not in the DB at all. Using `record_reference_to_unsolved_symbol(from_id, reference_type, file_id, start_line, start_column, end_line, end_column, hover_display="")` will record that `from_id` symbol has a reference of the given type at that location, but without a concrete `to_id` target ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=record_reference_to_unsolved_symbol)) ([Public API Reference - Numbat Documentation](https://quarkslab.github.io/numbat/public_api/#:~:text=Record%20a%20reference%20to%20an,unsolved%20symbol)). In Sourcetrail, this typically shows up as a reference arrow to a “?” node or similar. This approach requires less manual creation of symbols – you don’t create a `Println` node at all, for instance – but the user experience is a bit limited (they can see there is a call to something external, but can’t click through to see details about it). The hover text on the reference could be used to show the name of the external function (e.g., “Println”) so the user knows what’s being called. For example:  
  ```python
  from numbat import EdgeType  # EdgeType.CALL for call references
  db.record_reference_to_unsolved_symbol(caller_id, EdgeType.CALL,
      file_id, call_line, call_col, call_line, call_col+6, hover_display="Println (external)")
  ```  
  This would register a call edge originating from `caller_id` at the source location, pointing to an unsolved symbol named “Println”. It keeps the database cleaner (no extra nodes), but the UI will just indicate an unresolved reference.

**Which to choose?** If you prefer a more complete graph and the ability to navigate into external libraries (even if just to a stub node), use the **stub approach** with `is_indexed=False` symbols. This gives the user something to click on. If you want to keep the DB minimal, use **unsolved references** to simply mark external calls/usages. You can also mix both: e.g., stub out a few key standard library functions that are used often, and treat the rest as unsolved. Remember to always mark external symbols as non-indexed (so they appear distinct and do not confuse the user into thinking they were defined in the project). Non-indexed symbols show up with a gray icon in Sourcetrail, indicating they have no definition in the provided source ([Tutorial - Numbat Documentation](https://quarkslab.github.io/numbat/tutorial/#:~:text=,show%20as%20a%20grey%20shadow)).

Lastly, for external package imports, you might opt to create just a package node (as non-indexed) and use an import edge to it, without creating all its inner symbols. This still shows that “our package imports X package” on the graph. You can then decide case-by-case if anything inside X needs a stub.

## Handling Embedded Types and Composition in Go

In Go, a struct can embed other struct or interface types, granting the embedding struct the fields and methods of the embedded type. We model this in Numbat by creating an INHERITANCE reference (`record_ref_inheritance`). Specifically, the parser emits a reference with `RefType: "embeds"` whenever a struct or interface is embedded in another. During database generation, `generate_db.py` interprets `"embeds"` as an inheritance relationship (just like `"implements"`), so Sourcetrail displays the embedded type as a base class, reflecting Go’s composition through embedding.

## Conclusion  
By leveraging Numbat’s API thoughtfully, you can build a Sourcetrail database that mirrors Go’s structure and semantics. **Organize namespaces** to group symbols by module and package, **record symbols** with the appropriate type-specific calls to give them correct identity (struct vs interface vs function, etc.), and **enrich** those symbols with signatures or types using hover text. **Classify references** properly as calls, usages, inheritance (implements), or imports to let Sourcetrail display accurate relationships ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl · GitHub](https://github.com/takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=if%20ref_type%20%3D%3D%20)) ([gosrctrl/generate_db.py at main · takifouhal/gosrctrl/blob/main/generate_db.py#:~:text=ref_id%20%3D%20db)). And for anything outside your codebase, consider **external stubs or unsolved references** to maintain a useful but clean index of symbols. This approach will yield a more navigable and informative Sourcetrail view of Go projects, aligning with the improvements aimed for GoSrcCtrl’s output. Happy coding!
