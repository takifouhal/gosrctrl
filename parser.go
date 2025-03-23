package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// SymbolKind represents the category/kind of a symbol (package, type, var, func, etc.)
type SymbolKind string

const (
	SymbolKindPackage SymbolKind = "package"
	SymbolKindType    SymbolKind = "type"
	SymbolKindVar     SymbolKind = "var"
	SymbolKindField   SymbolKind = "field"
	SymbolKindFunc    SymbolKind = "func"
	SymbolKindMethod  SymbolKind = "method"
	SymbolKindConst   SymbolKind = "const"
)

// Symbol stores metadata about a single declared symbol in the codebase
type Symbol struct {
	ID          int        // Unique ID for this symbol
	Name        string     // Symbol name
	Kind        SymbolKind // e.g., package, type, var, const, func, method
	PackagePath string     // Full import path of the containing package
	File        string     // Source file where the symbol is declared
	Line        int        // Line number in the file
	Column      int        // Column number in the file
	Receiver    string     // Receiver type (for methods)
}

// Reference captures a usage relationship between two symbols.
type Reference struct {
	FromID  int    // ID of the symbol using another symbol
	ToID    int    // ID of the symbol being used
	File    string // Source file where the usage occurs
	Line    int    // Line number of the usage
	Column  int    // Column number of the usage
	RefType string // e.g. "usage" (could be extended to "call", etc.)
}

// LoadPackages loads and parses all Go packages beneath the specified path.
// It returns a slice of parsed packages (with syntax and type info) or an error.
func LoadPackages(path string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Dir: path,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Check for any loading errors in the loaded packages
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("errors encountered while loading packages")
	}

	return pkgs, nil
}

// ExtractSymbols traverses the provided Go packages and collects symbol definitions
// (functions, methods, types, variables, constants, etc.).
// Returns:
//  1. The slice of symbols
//  2. A map from types.Object -> Symbol.ID for quick lookup of definitions
//  3. A map from *packages.Package -> Symbol.ID for the package-level symbol
func ExtractSymbols(pkgs []*packages.Package) (
	[]Symbol,
	[]Reference,
	map[types.Object]int,
	map[*packages.Package]int,
) {
	var symbols []Symbol
	var importRefs []Reference
	objectToSymbol := make(map[types.Object]int)
	packageToSymbol := make(map[*packages.Package]int)

	var currentID int
	for _, pkg := range pkgs {
		// Create a symbol entry for the package itself
		currentID++
		packageSymbol := Symbol{
			ID:          currentID,
			Name:        pkg.Name,
			Kind:        SymbolKindPackage,
			PackagePath: pkg.PkgPath,
			File:        "",
			Line:        0,
			Column:      0,
			Receiver:    "",
		}
		symbols = append(symbols, packageSymbol)
		packageToSymbol[pkg] = currentID

		// For each defined identifier in pkg.TypesInfo.Defs, collect a symbol
		for id, obj := range pkg.TypesInfo.Defs {
			if obj == nil {
				// This identifier is not a definition
				continue
			}

			// Skip local variables or constants (non-struct fields) by checking parent scope
			switch v := obj.(type) {
			case *types.Var:
				if !v.IsField() && v.Parent() != pkg.Types.Scope() {
					continue
				}
			case *types.Const:
				if v.Parent() != pkg.Types.Scope() {
					continue
				}
			}

			pos := pkg.Fset.Position(id.Pos())
			if !pos.IsValid() {
				// Ignore definitions with invalid positions
				continue
			}

			kind := classifyObject(obj)
			receiver := ""

			// If it's a function, check if there's a receiver (making it a method)
			if fn, ok := obj.(*types.Func); ok {
				sig, _ := fn.Type().(*types.Signature)
				if sig != nil && sig.Recv() != nil {
					receiver = sig.Recv().Type().String()
				}
			}

			currentID++
			symbol := Symbol{
				ID:          currentID,
				Name:        obj.Name(),
				Kind:        kind,
				PackagePath: pkg.PkgPath,
				File:        pos.Filename,
				Line:        pos.Line,
				Column:      pos.Column,
				Receiver:    receiver,
			}

			// Special case for imports (types.PkgName)
			if pkgNameObj, ok := obj.(*types.PkgName); ok {
				symbol.PackagePath = pkgNameObj.Imported().Path()
				symbol.Name = pkgNameObj.Imported().Path()

				// Create an import reference from the current package to this imported package
				importRefs = append(importRefs, Reference{
					FromID: packageToSymbol[pkg],
					ToID:   currentID,
					File:   pos.Filename,
					Line:   pos.Line,
					Column: pos.Column,
					RefType: "import",
				})
			}

			symbols = append(symbols, symbol)
			objectToSymbol[obj] = currentID
		}
	}

	// Identify struct fields after collecting all symbols.
	// We'll map ID -> Object so we can discover the actual *types.TypeName objects, then
	// detect any underlying *types.Struct and mark its fields as "field".
	idToObject := make(map[int]types.Object)
	for obj, sid := range objectToSymbol {
		idToObject[sid] = obj
	}

	symbolIndexMap := make(map[int]int)
	for i, s := range symbols {
		symbolIndexMap[s.ID] = i
	}

	for _, s := range symbols {
		if s.Kind == SymbolKindType {
			obj := idToObject[s.ID]
			if typeName, ok := obj.(*types.TypeName); ok {
				if st, ok := typeName.Type().Underlying().(*types.Struct); ok {
					for i := 0; i < st.NumFields(); i++ {
						fieldObj := st.Field(i)
						if fieldID, found := objectToSymbol[fieldObj]; found {
							fieldIdx := symbolIndexMap[fieldID]
							symbols[fieldIdx].Kind = SymbolKindField
							// Reuse 'Receiver' to store the struct's full type string
							symbols[fieldIdx].Receiver = typeName.Type().String()
						}
					}
				}
			}
		}
	}

	return symbols, importRefs, objectToSymbol, packageToSymbol
}

// classifyObject inspects a types.Object to determine its symbol kind.
func classifyObject(obj types.Object) SymbolKind {
	switch o := obj.(type) {
	case *types.PkgName:
		return SymbolKindPackage
	case *types.TypeName:
		return SymbolKindType
	case *types.Const:
		return SymbolKindConst
	case *types.Var:
		return SymbolKindVar
	case *types.Func:
		// Determine if it's a method by checking for a receiver
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			return SymbolKindMethod
		}
		return SymbolKindFunc
	}
	// Fallback for unhandled cases
	return SymbolKindVar
}

// ExtractReferences finds usage relationships between symbols.
//  1. For each identifier in pkg.TypesInfo.Uses, if it's mapped to a known symbol (toID),
//  2. Determine the "from" symbol ID by discovering if that usage is inside a particular function.
//     If none is found, we use the package-level symbol ID.
//  3. Create a Reference with fromID, toID, and usage location.
func ExtractReferences(
	pkgs []*packages.Package,
	objectToSymbol map[types.Object]int,
	packageToSymbol map[*packages.Package]int,
) []Reference {
	var references []Reference

	for _, pkg := range pkgs {
		// Build a map of positions that correspond to a call expression
		callPositions := map[token.Pos]bool{}
		for _, f := range pkg.Syntax {
			if f == nil {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					switch fn := c.Fun.(type) {
					case *ast.Ident:
						callPositions[fn.Pos()] = true
					case *ast.SelectorExpr:
						callPositions[fn.Sel.Pos()] = true
					}
				}
				return true
			})
		}

		// The package-level symbol ID for references at the top level
		pkgSymbolID := packageToSymbol[pkg]

		// Build a map of filename -> *ast.File
		fileMap := make(map[string]*ast.File)
		for _, f := range pkg.Syntax {
			if f == nil {
				continue
			}
			fileName := pkg.Fset.File(f.Pos()).Name()
			fileMap[fileName] = f
		}

		// For each usage identified in pkg.TypesInfo.Uses
		for id, obj := range pkg.TypesInfo.Uses {
			toID, ok := objectToSymbol[obj]
			if !ok {
				// Usage is referencing an external or unknown symbol; skip it
				continue
			}
			pos := pkg.Fset.Position(id.Pos())
			if !pos.IsValid() {
				continue
			}

			// Determine fromID
			fromID := pkgSymbolID
			fileAst := fileMap[pos.Filename]
			if fileAst != nil {
				funcID := findEnclosingFunctionID(id.Pos(), fileAst, pkg, objectToSymbol)
				if funcID != -1 {
					fromID = funcID
				}
			}

			// Decide reference type
			refType := "usage"
			if callPositions[id.Pos()] {
				refType = "call"
			}

			// Build the reference
			r := Reference{
				FromID:  fromID,
				ToID:    toID,
				File:    pos.Filename,
				Line:    pos.Line,
				Column:  pos.Column,
				RefType: refType,
			}
			references = append(references, r)
		}
	}

	return references
}

// ExtractTypeRelations detects:
// 1) "struct X implements interface Y"
// 2) "struct A embeds struct B"
//
// It returns references with RefType = "implements" or "embeds".
func ExtractTypeRelations(
	pkgs []*packages.Package,
	symbols []Symbol,
	objectToSymbol map[types.Object]int,
	packageToSymbol map[*packages.Package]int,
) []Reference {

	var out []Reference

	// Step 1: Build a list of all (struct, structID, types.Type) and (interface, interfaceID, *types.Interface)
	typeEntry := []struct {
		id   int
		obj  types.Object
		typ  types.Type
	}{}
	interfaceEntry := []struct {
		id        int
		obj       types.Object
		ifaceType *types.Interface
	}{}

	// We already have an idToObject map internally, let's build it again or on the fly:
	idToObject := make(map[int]types.Object, len(objectToSymbol))
	for obj, sid := range objectToSymbol {
		idToObject[sid] = obj
	}

	// Identify interface vs struct from the known symbols
	for _, sym := range symbols {
		if sym.Kind != SymbolKindType {
			continue
		}
		obj := idToObject[sym.ID]
		typeName, _ := obj.(*types.TypeName)
		if typeName == nil {
			continue
		}
		typ := typeName.Type()
		// Check if interface
		if iface, ok := typ.Underlying().(*types.Interface); ok {
			interfaceEntry = append(interfaceEntry, struct {
				id        int
				obj       types.Object
				ifaceType *types.Interface
			}{
				id:        sym.ID,
				obj:       obj,
				ifaceType: iface,
			})
		} else {
			// Must be a struct or other concrete type
			typeEntry = append(typeEntry, struct {
				id   int
				obj  types.Object
				typ  types.Type
			}{
				id:  sym.ID,
				obj: obj,
				typ: typ,
			})
		}
	}

	// Step 2: For each interface, check which concrete types implement it.
	// types.Implements(concrete, interface) returns true if concrete implements interface
	for _, ifaceE := range interfaceEntry {
		ifaceType := ifaceE.ifaceType
		for _, conc := range typeEntry {
			// We check both T and *T (pointer) for the method set
			if types.Implements(conc.typ, ifaceType) || types.Implements(types.NewPointer(conc.typ), ifaceType) {
				r := Reference{
					FromID:  conc.id,
					ToID:    ifaceE.id,
					RefType: "implements",
				}
				out = append(out, r)
			}
		}
	}

	// Step 3: For each struct, detect embedded fields
	for _, conc := range typeEntry {
		under := conc.typ.Underlying()
		st, ok := under.(*types.Struct)
		if !ok {
			continue
		}
		// For each field, check if it's embedded
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if f.Embedded() {
				// If we know the field's symbol ID, create an "embeds" reference
				if embedID, found := objectToSymbol[f]; found {
					// "FromID" = struct, "ToID" = embedded struct
					r := Reference{
						FromID:  conc.id,
						ToID:    embedID,
						RefType: "embeds",
					}
					out = append(out, r)
				}
			}
		}
	}

	return out
}

// findEnclosingFunctionID checks whether a position belongs to the body of a known function/method.
// If found, returns that function's symbol ID; otherwise returns -1.
func findEnclosingFunctionID(
	pos token.Pos,
	fileAst *ast.File,
	pkg *packages.Package,
	objectToSymbol map[types.Object]int,
) int {
	for _, decl := range fileAst.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Check if pos is inside the function's body
			if funcDecl.Body != nil && funcDecl.Body.Pos() <= pos && pos < funcDecl.Body.End() {
				// This identifier is within funcDecl's body
				fnObj := pkg.TypesInfo.Defs[funcDecl.Name]
				if fnObj != nil {
					if fnID, ok := objectToSymbol[fnObj]; ok {
						return fnID
					}
				}
			}
		}
	}
	return -1
}