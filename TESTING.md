# GoSrcCtrl - Output Visualization & Quality Testing

This document provides instructions to test and validate the Sourcetrail database produced by GoSrcCtrl. By following these steps, you can ensure your generated `.srctrldb` file is navigable and accurately represents your Go code’s structure and references.

---

## 1. Building & Running GoSrcCtrl

1. **Install dependencies** (Go 1.18+ and Python 3.8+):
   ```bash
   make setup-go
   make setup-python
   ```
   or do this manually by running:
   ```bash
   go get golang.org/x/tools/go/packages
   python3 -m venv venv
   source venv/bin/activate
   pip install --upgrade pip
   pip install -r requirements.txt
   ```

2. **Build GoSrcCtrl**:
   ```bash
   make build
   ```
   This produces the `gosrctrl` binary locally.

3. **Run GoSrcCtrl** to generate a `.srctrldb`:
   ```bash
   ./gosrctrl -path /path/to/your/go/project -out myoutput.srctrldb
   ```
   Alternatively:
   ```bash
   gosrctrl -path /path/to/your/go/project -out myoutput.srctrldb
   ```
   after installing it in your Go bin path (`make install`).

---

## 2. Opening the Sourcetrail Database

1. **Launch Sourcetrail**, and use **File > Open Project** to open `myoutput.srctrldb`.
2. Ensure that the project loads without errors or warnings. If there are errors, check the GoSrcCtrl console output or logs in Sourcetrail for hints (e.g., missing references).

---

## 3. Verifying Hierarchy

GoSrcCtrl aims to organize symbols under:
- **Module** (from `go.mod`),
- **Package** (Go package import path),
- **Types** (struct, interface, type alias, etc.),
- **Functions and Methods**,
- **Variables and Constants**.

When you expand the nodes in Sourcetrail, verify:

1. **Module Nodes**: Appear as top-level namespaces for each module discovered in `go.mod`. The first (main) module should be flagged as `is_indexed` and show a normal icon. Any external references appear in a special `EXTERNAL` node or have separate modules with `is_indexed = false`.
2. **Package Nodes**: Each Go package belonging to your main module is nested under the module node. Sub-packages might be further nested, reflecting their relative import paths (e.g., `module/foo`, `module/foo/bar`).
3. **Types & Methods**: Inside each package node, check that your structs, interfaces, or type aliases appear. Methods (including those with pointer receivers, e.g. `(*Struct)`) should appear under their corresponding receiver type, while stand-alone functions are attached directly to the package node. This ensures object-oriented relationships are visible in the graph.
4. **Variables & Constants**:

---

## 4. Inspecting Symbol Hover Text

Select (or hover over) functions, methods, and interface symbols to confirm:

- **Signatures** or descriptions appear in a hover tooltip (e.g., `"func Add(x int) int"` for a function).
- **Interface** nodes display something like `"type MyInterface interface {...}"`, or a postfix `(interface)` indicating it’s an interface type.
- **Struct** nodes can contain a snippet of their definition if provided in the hover text.

---

## 5. Checking References (Edges)

### 5.1 Calls
- Write a small code snippet where `func A` calls `func B`. Verify that in Sourcetrail:
  1. There is a **Call Edge** from `A` to `B`.
  2. Hovering the edge or symbol location shows the file/line details.

### 5.2 Usage
- If `func A` reads or writes a package-level variable or a struct field, check for a **Usage Edge** from `A` to that variable or field symbol.
- Confirm that trivial references (like a function referencing its own package symbol) are trimmed if that’s a desired filter.

### 5.3 Imports
- For a package that imports another local or external package, confirm an **Import Edge** from your package node to the imported package node.

### 5.4 Interface Implementations & Struct Embedding
- If you have a struct `S` implementing interface `I`, confirm an **Inheritance/Implements** relationship from `S` to `I` in the graph. It may appear as an inheritance arrow (this is how Numbat displays “implements”).
- If you embed a struct `B` inside another struct `A`, confirm an **Inheritance** arrow from `A` to `B`.

### 5.5 External Library Stubs
- If your code calls `fmt.Println` or other external library functions, check that a minimal external node is created (if you are creating stubs). It should be marked as not indexed (gray/dim icon). There should be a **Call** edge from your local function to the external stub symbol.

---

## 6. Confirming Reduced Noise

One of the final goals is to ensure references are not overpopulated with trivial usage edges. Check for:

- **Package-level self-references** or function->package references that you do **not** want. They should be absent (filtered out or minimized).
- **Unnecessary references** (e.g., every single mention of local variables) should not clutter the graph. Instead, focus on references that help you navigate the code’s architecture: function calls, global var usage, interface implementations, etc.

---

## 7. Performance & Larger Codebases

Try running GoSrcCtrl on a moderately larger repository:

1. **Performance**: The tool should complete in a reasonable time given the standard `golang.org/x/tools/go/packages` approach.
2. **Database Size**: Inspect the size of the `.srctrldb`. If it’s excessively large, consider refining reference filters or skipping unneeded external stubs.

---

## 8. Troubleshooting

- **Missing Symbol or Edge**: Check the logs printed by `gosrctrl` to see if an object was recognized. Confirm that `LoadPackages` includes all needed subdirectories and that no errors appeared during parse.
- **Sourcetrail Errors**: If Sourcetrail complains about invalid references, ensure that for each `RefType` there’s a valid `from` and `to` symbol. Invalid file paths or line numbers can also cause warnings.
- **External Dependencies**: If you see a large number of external stubs, refine your code in `parser.go` or `generate_db.py` to skip or unify them.

---

## 9. Conclusion

By running these checks, you can verify that your generated `.srctrldb` is well-structured, easy to navigate, and accurately models the Go code’s real architecture. This final step ensures that **GoSrcCtrl** meets its goal of creating a **Sourcetrail** project that is both rich in detail and free of unnecessary clutter.

For further improvements, consider:
- More granular reference categorization (distinguish reads vs writes).
- Thorough coverage of external packages or more minimal stubbing.
- Additional synergy with other analysis tools, etc.