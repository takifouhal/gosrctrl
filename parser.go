package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// SymbolKind represents the category/kind of a symbol (package, type, var, func, etc.)
type SymbolKind string

const (
	SymbolKindPackage   SymbolKind = "package"
	SymbolKindType      SymbolKind = "type"
	SymbolKindStruct    SymbolKind = "struct"
	SymbolKindVar       SymbolKind = "var"
	SymbolKindField     SymbolKind = "field"
	SymbolKindFunc      SymbolKind = "func"
	SymbolKindMethod    SymbolKind = "method"
	SymbolKindConst     SymbolKind = "const"
	SymbolKindInterface SymbolKind = "interface"
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
	Sig         string     // Signature or definition snippet (for funcs, interfaces, etc.)
	External    bool       // Marks stub symbols for external code

	ParentID int // Optional parent symbol ID (e.g., for field->struct)
}

// Reference captures a usage relationship between two symbols.
type Reference struct {
	FromID  int    // ID of the symbol using another symbol
	ToID    int    // ID of the symbol being used
	File    string // Source file where the usage occurs
	Line    int    // Line number of the usage
	Column  int    // Column number of the usage
	RefType string // e.g. "usage" or "call", "implements", "embeds", etc.
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

// getOrCreateStubForExternal ensures we have a Symbol ID for the given object,
// creating an external symbol stub if it doesn't exist. Returns the symbol ID
// and a boolean indicating success. If the object cannot be resolved to a package,
// returns -1, false.
func getOrCreateStubForExternal(
	obj types.Object,
	objectToSymbol map[types.Object]int,
	symbols *[]Symbol,
	maxID *int,
) (int, bool) {

	if existingID, found := objectToSymbol[obj]; found {
		return existingID, true
	}

	objPkg := obj.Pkg()
	if objPkg == nil {
		// If we can’t determine a package, we can’t create a meaningful stub.
		return -1, false
	}

	// Bump ID for the new symbol
	*maxID++
	stubKind := classifyObject(obj)

	// Adjust if the object is a named type that’s actually struct or interface
	if stubKind == SymbolKindType {
		if named, okType := obj.Type().(*types.Named); okType {
			if _, isIface := named.Underlying().(*types.Interface); isIface {
				stubKind = SymbolKindInterface
			} else if _, isStruct := named.Underlying().(*types.Struct); isStruct {
				stubKind = SymbolKindStruct
			}
		}
	}

	stubSym := Symbol{
		ID:          *maxID,
		Name:        obj.Name(),
		Kind:        stubKind,
		PackagePath: objPkg.Path(),
		External:    true,
	}

	*symbols = append(*symbols, stubSym)
	objectToSymbol[obj] = *maxID

	return *maxID, true
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
			Sig:         "",
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
				Sig:         "",
			}

			// Now set the signature for functions
			if fn, ok := obj.(*types.Func); ok {
				sig, _ := fn.Type().(*types.Signature)
				if sig != nil {
					// Build a formatted signature string, including receiver if present
					symbol.Sig = buildFuncSignature(fn, sig)
				}
			}

			// Special case for imports (types.PkgName)
			if pkgNameObj, ok := obj.(*types.PkgName); ok {
				symbol.PackagePath = pkgNameObj.Imported().Path()
				symbol.Name = pkgNameObj.Imported().Path()

				// Create an import reference from the current package to this imported package
				importRefs = append(importRefs, Reference{
					FromID:  packageToSymbol[pkg],
					ToID:    currentID,
					File:    pos.Filename,
					Line:    pos.Line,
					Column:  pos.Column,
					RefType: "import",
				})
			}

			symbols = append(symbols, symbol)
			objectToSymbol[obj] = currentID
		}
	}

	// Identify struct fields after collecting all symbols.
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
				under := typeName.Type().Underlying()

				if st, ok := under.(*types.Struct); ok {
					// Mark this symbol as a struct
					symbols[symbolIndexMap[s.ID]].Kind = SymbolKindStruct

					// Now ensure all fields (including embedded) have Symbol entries
					for i := 0; i < st.NumFields(); i++ {
						fieldObj := st.Field(i)
						if fieldID, found := objectToSymbol[fieldObj]; found {
							fieldIdx := symbolIndexMap[fieldID]
							symbols[fieldIdx].Kind = SymbolKindField
							// Reuse 'Receiver' to store the struct's full type string
							symbols[fieldIdx].Receiver = typeName.Type().String()
							// Also set ParentID to the struct's ID
							symbols[fieldIdx].ParentID = s.ID
						} else {
							// Create a new symbol for an embedded or otherwise missing field
							currentID++
							newField := Symbol{
								ID:          currentID,
								Name:        fieldObj.Name(),
								Kind:        SymbolKindField,
								PackagePath: s.PackagePath,
								File:        "",
								Line:        0,
								Column:      0,
								Receiver:    typeName.Type().String(),
								ParentID:    s.ID,
							}
							symbols = append(symbols, newField)
							objectToSymbol[fieldObj] = currentID
							symbolIndexMap[currentID] = len(symbols) - 1
						}
					}
				} else if iface, ok := under.(*types.Interface); ok {
					symbols[symbolIndexMap[s.ID]].Kind = SymbolKindInterface
					symbols[symbolIndexMap[s.ID]].Sig = buildInterfaceSignature(typeName, iface)
				}
				// Else it remains SymbolKindType for other named types (aliases, etc.)
			}
		}
	}

	return symbols, importRefs, objectToSymbol, packageToSymbol
}

// buildFuncSignature creates a hover text like: func (Receiver) Name(param1 Type1, param2 Type2) (ret Type)
func buildFuncSignature(fn *types.Func, sig *types.Signature) string {
	recv := ""
	if sig.Recv() != nil {
		recvType := sig.Recv().Type().String()
		recv = "(" + recvType + ") "
	}

	params := ""
	for i := 0; i < sig.Params().Len(); i++ {
		p := sig.Params().At(i)
		if i > 0 {
			params += ", "
		}
		params += p.Name() + " " + p.Type().String()
	}

	results := ""
	if sig.Results().Len() > 0 {
		if sig.Results().Len() == 1 {
			r := sig.Results().At(0)
			results = r.Type().String()
		} else {
			var rr []string
			for i := 0; i < sig.Results().Len(); i++ {
				r := sig.Results().At(i)
				rr = append(rr, r.Name()+" "+r.Type().String())
			}
			results = "(" + strings.Join(rr, ", ") + ")"
		}
	}

	signature := "func " + recv + fn.Name() + "(" + params + ")"
	if results != "" {
		signature += " " + results
	}
	return signature
}

// buildInterfaceSignature creates a short snippet for an interface symbol
// e.g. "type MyInterface interface { Method1(...) ... }"
func buildInterfaceSignature(typeName *types.TypeName, iface *types.Interface) string {
	var sb strings.Builder
	sb.WriteString("type ")
	sb.WriteString(typeName.Name())
	sb.WriteString(" interface {\n")
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		sig, _ := m.Type().(*types.Signature)
		if sig != nil {
			// buildFuncSignature includes "func " prefix, which we don't need in an interface block
			sb.WriteString("    ")
			sb.WriteString(buildFuncSignature(m, sig)[5:]) // strip off "func "
			sb.WriteString("\n")
		}
	}
	sb.WriteString("}")
	return sb.String()
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
//     otherwise attempt to create an external stub symbol if from an external package.
//  2. Determine the "from" symbol ID by discovering if that usage is inside a particular function.
//     If none is found, we use the package-level symbol ID.
//  3. Create a Reference with fromID, toID, and usage location.
func ExtractReferences(
	pkgs []*packages.Package,
	symbols []Symbol,
	objectToSymbol map[types.Object]int,
	packageToSymbol map[*packages.Package]int,
) []Reference {

	var references []Reference

	maxID := getMaxSymbolID(objectToSymbol)

	// For each package, gather call and write positions by traversing AST
	for _, pkg := range pkgs {
		callPositions := map[token.Pos]bool{}
		writePositions := map[token.Pos]bool{}

		for _, f := range pkg.Syntax {
			if f == nil {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				// Detect calls
				if c, ok := n.(*ast.CallExpr); ok {
					switch fn := c.Fun.(type) {
					case *ast.Ident:
						callPositions[fn.Pos()] = true
					case *ast.SelectorExpr:
						callPositions[fn.Sel.Pos()] = true
					}
				}

				// Detect assignments
				if assign, ok := n.(*ast.AssignStmt); ok {
					for _, lhs := range assign.Lhs {
						if ident, ok := lhs.(*ast.Ident); ok {
							writePositions[ident.Pos()] = true
						}
					}
				}
				return true
			})
		}

		pkgSymbolID := packageToSymbol[pkg]
		fileMap := make(map[string]*ast.File)
		for _, f := range pkg.Syntax {
			if f == nil {
				continue
			}
			fileName := pkg.Fset.File(f.Pos()).Name()
			fileMap[fileName] = f
		}

		for id, obj := range pkg.TypesInfo.Uses {
			toID, ok := objectToSymbol[obj]
			if !ok {
				// Attempt to create external stub symbol if object from external package
				newID, success := getOrCreateStubForExternal(obj, objectToSymbol, &symbols, &maxID)
				if !success {
					// If we can't create an external stub, skip
					continue
				}
				toID = newID
			}
			pos := pkg.Fset.Position(id.Pos())
			if !pos.IsValid() {
				continue
			}

			// fromID defaults to the package-level ID
			fromID := pkgSymbolID
			fileAst := fileMap[pos.Filename]
			if fileAst != nil {
				funcID := findEnclosingFunctionID(id.Pos(), fileAst, pkg, objectToSymbol)
				if funcID != -1 {
					fromID = funcID
				}
			}

			refType := "usage"
			if callPositions[id.Pos()] {
				refType = "call"
			} else if writePositions[id.Pos()] {
				refType = "write"
			}

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
// 3) "interface A embeds interface B"
// 4) field->type usage references
// 5) func->param/return type usage references
// Returns references with RefType = "implements" or "embeds" or "usage".
func ExtractTypeRelations(
	pkgs []*packages.Package,
	symbols []Symbol,
	objectToSymbol map[types.Object]int,
	packageToSymbol map[*packages.Package]int,
) []Reference {

	var out []Reference

	maxID := getMaxSymbolID(objectToSymbol)

	idToObject := make(map[int]types.Object, len(objectToSymbol))
	for obj, sid := range objectToSymbol {
		idToObject[sid] = obj
	}

	// We'll gather interface and type entries
	typeEntry := []struct {
		id  int
		obj types.Object
		typ types.Type
	}{}
	interfaceEntry := []struct {
		id        int
		obj       types.Object
		ifaceType *types.Interface
	}{}

	for _, sym := range symbols {
		if sym.Kind != SymbolKindType && sym.Kind != SymbolKindInterface {
			continue
		}
		obj := idToObject[sym.ID]
		typeName, _ := obj.(*types.TypeName)
		if typeName == nil {
			continue
		}
		typ := typeName.Type()
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
			typeEntry = append(typeEntry, struct {
				id  int
				obj types.Object
				typ types.Type
			}{
				id:  sym.ID,
				obj: obj,
				typ: typ,
			})
		}
	}

	// Helper for recording "usage" references from a symbol to named types
	var addTypeUsageRef func(fromSid int, t types.Type, out []Reference) []Reference
	addTypeUsageRef = func(fromSid int, t types.Type, out []Reference) []Reference {
		switch tt := t.(type) {
		case *types.Pointer:
			return addTypeUsageRef(fromSid, tt.Elem(), out)
		case *types.Named:
			if typeSid, found := objectToSymbol[tt.Obj()]; found {
				out = append(out, Reference{
					FromID:  fromSid,
					ToID:    typeSid,
					RefType: "usage",
				})
			} else {
				// Possibly create an external stub
				newID, success := getOrCreateStubForExternal(tt.Obj(), objectToSymbol, &symbols, &maxID)
				if success {
					out = append(out, Reference{
						FromID:  fromSid,
						ToID:    newID,
						RefType: "usage",
					})
				}
			}
		case *types.Slice:
			return addTypeUsageRef(fromSid, tt.Elem(), out)
		case *types.Array:
			return addTypeUsageRef(fromSid, tt.Elem(), out)
		case *types.Chan:
			return addTypeUsageRef(fromSid, tt.Elem(), out)
		case *types.Map:
			out = addTypeUsageRef(fromSid, tt.Key(), out)
			return addTypeUsageRef(fromSid, tt.Elem(), out)
		}
		return out
	}

	// 1) For each interface, check which concrete types implement it
	for _, ifaceE := range interfaceEntry {
		ifaceType := ifaceE.ifaceType

		for _, conc := range typeEntry {
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

	// 2) For each struct, detect embedded fields
	for _, conc := range typeEntry {
		under := conc.typ.Underlying()
		st, ok := under.(*types.Struct)
		if !ok {
			continue
		}
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if f.Embedded() {
				et := f.Type()
				if pt, ok := et.(*types.Pointer); ok {
					et = pt.Elem()
				}
				if named, ok := et.(*types.Named); ok {
					embedObj := named.Obj()
					embedID, found := objectToSymbol[embedObj]
					if !found {
						newID, success := getOrCreateStubForExternal(embedObj, objectToSymbol, &symbols, &maxID)
						if !success {
							continue
						}
						embedID = newID
					}
					out = append(out, Reference{
						FromID:  conc.id,
						ToID:    embedID,
						RefType: "embeds",
					})
				}
			}
		}
	}

	// 3) For each interface, detect embedded interfaces
	for _, ifaceE := range interfaceEntry {
		ifa := ifaceE.ifaceType
		for i := 0; i < ifa.NumEmbeddeds(); i++ {
			embeddedT := ifa.EmbeddedType(i)
			if named, ok := embeddedT.(*types.Named); ok {
				if _, ok2 := named.Underlying().(*types.Interface); ok2 {
					embedObj := named.Obj()
					embedID, found := objectToSymbol[embedObj]
					if !found {
						newID, success := getOrCreateStubForExternal(embedObj, objectToSymbol, &symbols, &maxID)
						if !success {
							continue
						}
						embedID = newID
					}
					out = append(out, Reference{
						FromID:  ifaceE.id,
						ToID:    embedID,
						RefType: "embeds",
					})
				}
			}
		}
	}

	// 4) Create usage references from each field symbol to its type (if named).
	for _, sym := range symbols {
		if sym.Kind != SymbolKindField {
			continue
		}
		fieldObj := idToObject[sym.ID]
		if fieldVar, ok := fieldObj.(*types.Var); ok {
			fieldType := fieldVar.Type()
			out = addTypeUsageRef(sym.ID, fieldType, out)
		}
	}

	// 5) Create usage references from each func/method to its param and return types.
	for _, sym := range symbols {
		if sym.Kind != SymbolKindFunc && sym.Kind != SymbolKindMethod {
			continue
		}
		fnObj := idToObject[sym.ID]
		fn, ok := fnObj.(*types.Func)
		if !ok {
			continue
		}
		sig, _ := fn.Type().(*types.Signature)
		if sig == nil {
			continue
		}
		// Parameters
		for i := 0; i < sig.Params().Len(); i++ {
			paramType := sig.Params().At(i).Type()
			out = addTypeUsageRef(sym.ID, paramType, out)
		}
		// Results
		for i := 0; i < sig.Results().Len(); i++ {
			resultType := sig.Results().At(i).Type()
			out = addTypeUsageRef(sym.ID, resultType, out)
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

func getMaxSymbolID(objectToSymbol map[types.Object]int) int {
	var maxID int
	for _, sid := range objectToSymbol {
		if sid > maxID {
			maxID = sid
		}
	}
	return maxID
}