package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
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

// Global configuration options
var (
	// EnableEnhancedExternalTypes controls whether to use enhanced external type processing
	// which creates more stubs for external types and their fields/methods
	EnableEnhancedExternalTypes bool = true

	// VerboseLogging controls whether to print detailed debug information
	VerboseLogging bool = false

	// IgnoreCompileErrors controls whether to continue parsing even if there are compile errors
	IgnoreCompileErrors bool = false

	// IncludeStdLib controls whether to process standard library packages
	IncludeStdLib bool = false

	// IncludePrivate controls whether to process unexported symbols
	IncludePrivate bool = false
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
// When includeTests is true, test packages and testdata directories are processed.
func LoadPackages(path string, includeTests bool) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Dir: path,
		// Include test packages when requested
		Tests: includeTests,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Check for any loading errors in the loaded packages
	if errorCount := packages.PrintErrors(pkgs); errorCount > 0 {
		if !IgnoreCompileErrors {
			return nil, fmt.Errorf("errors encountered while loading packages")
		}
		fmt.Printf("Warning: %d errors encountered while loading packages, but proceeding anyway due to -ignore-errors flag\n", errorCount)
	}

	return pkgs, nil
}

// IsStdLibPkg checks if a package path is from the Go standard library
func IsStdLibPkg(pkgPath string) bool {
	// Standard library packages have no "." in their path
	return !strings.Contains(pkgPath, ".")
}

// getOrCreateStubForExternal creates a stub for an external type if it doesn't exist
func getOrCreateStubForExternal(
	obj types.Object,
	objectToSymbol map[types.Object]int,
	symbolsPtr *[]Symbol,
	maxID *int,
) (int, bool) {
	// If the object is already registered, return its ID
	if sid, found := objectToSymbol[obj]; found {
		return sid, true
	}

	// Only Named types are eligible
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return 0, false
	}

	// Skip if not enabled or if the object doesn't have a package
	if !EnableEnhancedExternalTypes || obj.Pkg() == nil {
		return 0, false
	}

	// Get package information
	pkgPath := obj.Pkg().Path()

	// Skip standard library if option set
	if IsStdLibPkg(pkgPath) && !IncludeStdLib {
		if VerboseLogging {
			fmt.Printf("[DEBUG EXTERNAL] Skipping stdlib type %s.%s\n", pkgPath, obj.Name())
		}
		return 0, false
	}

	// Only process exported types unless configured otherwise
	if !IncludePrivate && !obj.Exported() {
		if VerboseLogging {
			fmt.Printf("[DEBUG EXTERNAL] Skipping unexported type %s.%s\n", pkgPath, obj.Name())
		}
		return 0, false
	}

	// Create the new symbol for the external type
	newSid := *maxID + 1
	*maxID = newSid
	typeName := obj.Name()

	// Check the underlying type to determine symbol kind
	var kind SymbolKind
	var sig string

	switch named.Underlying().(type) {
	case *types.Struct:
		kind = SymbolKindStruct
		sig = fmt.Sprintf("struct %s.%s { ... }", pkgPath, typeName)
	case *types.Interface:
		kind = SymbolKindInterface
		sig = fmt.Sprintf("interface %s.%s { ... }", pkgPath, typeName)
	default:
		kind = SymbolKindType
		sig = fmt.Sprintf("type %s.%s", pkgPath, typeName)
	}

	// Create the symbol and register it
	symbol := Symbol{
		ID:          newSid,
		Name:        typeName,
		Kind:        kind,
		PackagePath: pkgPath,
		File:        fmt.Sprintf("external://%s/%s.go", pkgPath, strings.ToLower(typeName)),
		Line:        1,
		Column:      1,
		Sig:         sig,
		External:    true,
		ParentID:    0,
	}

	*symbolsPtr = append(*symbolsPtr, symbol)
	objectToSymbol[obj] = newSid

	if VerboseLogging {
		fmt.Printf("[DEBUG EXTERNAL] Created external symbol ID %d for %s.%s\n",
			newSid, pkgPath, obj.Name())
	}

	return newSid, true
}

// formatExternalTypeSignature creates a more descriptive signature for external types
func formatExternalTypeSignature(obj types.Object) string {
	if obj.Pkg() == nil {
		return obj.Name()
	}

	switch t := obj.(type) {
	case *types.TypeName:
		if named, ok := t.Type().(*types.Named); ok {
			if st, ok := named.Underlying().(*types.Struct); ok {
				// For structs, include basic field information
				numFields := st.NumFields()
				sig := fmt.Sprintf("struct %s.%s {", obj.Pkg().Path(), obj.Name())
				if numFields > 0 {
					sig += " ... " + strconv.Itoa(numFields) + " fields"
				}
				sig += " }"
				return sig
			} else if iface, ok := named.Underlying().(*types.Interface); ok {
				// For interfaces, include method information
				numMethods := iface.NumMethods()
				sig := fmt.Sprintf("interface %s.%s {", obj.Pkg().Path(), obj.Name())
				if numMethods > 0 {
					sig += " ... " + strconv.Itoa(numMethods) + " methods"
				}
				sig += " }"
				return sig
			}
		}
	}

	// Default format: package.Symbol
	return fmt.Sprintf("%s.%s", obj.Pkg().Path(), obj.Name())
}

// extractExternalTypeMembers attempts to extract key fields/methods from external types
// to create stubs for them as well, improving visualization
func extractExternalTypeMembers(
	named *types.Named,
	objectToSymbol map[types.Object]int,
	symbols *[]Symbol,
	maxID *int,
	parentID int,
) {
	// For struct types, try to add field stubs
	if st, ok := named.Underlying().(*types.Struct); ok {
		for i := 0; i < st.NumFields(); i++ {
			field := st.Field(i)
			if field.Exported() {
				// Only create stubs for exported fields of external types
				*maxID++
				fieldSym := Symbol{
					ID:          *maxID,
					Name:        field.Name(),
					Kind:        SymbolKindField,
					PackagePath: named.Obj().Pkg().Path(),
					ParentID:    parentID,
					External:    true,
				}
				*symbols = append(*symbols, fieldSym)
				objectToSymbol[field] = *maxID

				// Also add usage reference from field to its type
				fieldType := field.Type()
				addTypeUsageRef(fieldSym.ID, fieldType, nil, objectToSymbol, symbols, maxID)
			}
		}
	}

	// Add method stubs for both struct and interface types
	// First check interface methods
	if iface, ok := named.Underlying().(*types.Interface); ok {
		for i := 0; i < iface.NumMethods(); i++ {
			method := iface.Method(i)
			if method.Exported() {
				*maxID++
				methodSym := Symbol{
					ID:          *maxID,
					Name:        method.Name(),
					Kind:        SymbolKindMethod,
					PackagePath: named.Obj().Pkg().Path(),
					ParentID:    parentID,
					External:    true,
				}
				*symbols = append(*symbols, methodSym)
				objectToSymbol[method] = *maxID
			}
		}
	}

	// Then add methods declared on the type
	for i := 0; i < named.NumMethods(); i++ {
		method := named.Method(i)
		if method.Exported() {
			*maxID++
			methodSym := Symbol{
				ID:          *maxID,
				Name:        method.Name(),
				Kind:        SymbolKindMethod,
				PackagePath: named.Obj().Pkg().Path(),
				ParentID:    parentID,
				External:    true,
			}
			*symbols = append(*symbols, methodSym)
			objectToSymbol[method] = *maxID
		}
	}
}

// ExtractSymbols traverses the provided Go packages and collects symbol definitions
// (functions, methods, types, variables, constants, etc.).
// Returns:
//  1. The slice of symbols
//  2. A slice of "import" references
//  3. A map from types.Object -> Symbol.ID for quick lookup of definitions
//  4. A map from *packages.Package -> Symbol.ID for the package-level symbol
func ExtractSymbols(pkgs []*packages.Package) (
	[]Symbol,
	[]Reference,
	map[types.Object]int,
	map[*packages.Package]int,
) {
	// We'll collect symbols in multiple passes so that struct/interface
	// symbols are always created before their methods are attached.

	var symbols []Symbol
	var importRefs []Reference
	objectToSymbol := make(map[types.Object]int)
	packageToSymbol := make(map[*packages.Package]int)
	var currentID int

	// ------------------------------------------------------------
	// Pass 0: create a package symbol for each package
	for _, pkg := range pkgs {
		currentID++
		pkgSym := Symbol{
			ID:          currentID,
			Name:        pkg.Name,
			Kind:        SymbolKindPackage,
			PackagePath: pkg.PkgPath,
		}
		symbols = append(symbols, pkgSym)
		packageToSymbol[pkg] = currentID
	}

	// ------------------------------------------------------------
	// Pass 1: register all "type" definitions (TypeName), ignoring methods for now.
	for _, pkg := range pkgs {
		for id, obj := range pkg.TypesInfo.Defs {
			if obj == nil {
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

			// We only handle type declarations here (e.g. *types.TypeName).
			if typeName, ok := obj.(*types.TypeName); ok {
				currentID++
				symbol := Symbol{
					ID:          currentID,
					Name:        typeName.Name(),
					Kind:        SymbolKindType, // We'll refine struct/interface later
					PackagePath: pkg.PkgPath,
					File:        pos.Filename,
					Line:        pos.Line,
					Column:      pos.Column,
				}
				// The parent is the package symbol
				if pkgID, found := packageToSymbol[pkg]; found {
					symbol.ParentID = pkgID
				}
				symbols = append(symbols, symbol)
				objectToSymbol[obj] = currentID
			}
		}
	}

	// ------------------------------------------------------------
	// Pass 2: register non-type defs (func, var, const, also *types.PkgName for imports).
	for _, pkg := range pkgs {
		pkgSymID := packageToSymbol[pkg]
		for id, obj := range pkg.TypesInfo.Defs {
			if obj == nil {
				continue
			}

			// We skip type names here, because they were already handled
			if _, isTypeName := obj.(*types.TypeName); isTypeName {
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
				continue
			}

			kind := classifyObject(obj)
			currentID++
			symbol := Symbol{
				ID:          currentID,
				Name:        obj.Name(),
				Kind:        kind,
				PackagePath: pkg.PkgPath,
				File:        pos.Filename,
				Line:        pos.Line,
				Column:      pos.Column,
			}
			// Default parent is the package symbol
			symbol.ParentID = pkgSymID

			// Special handling for functions/methods
			if fn, ok := obj.(*types.Func); ok {
				if sig, _ := fn.Type().(*types.Signature); sig != nil {
					symbol.Sig = buildFuncSignature(fn, sig)
					fmt.Printf("[DEBUG FUNCTIONS] Found function: %s with signature %s\n", fn.Name(), symbol.Sig)
					// If it's a method (has receiver), try to find the parent type.
					if sig.Recv() != nil {
						theType := sig.Recv().Type()
						if p, ok := theType.(*types.Pointer); ok {
							theType = p.Elem()
						}
						if named, ok := theType.(*types.Named); ok {
							fmt.Printf("[DEBUG FUNCTIONS] It's a method with named receiver: %s\n", named.Obj().Name())
							if pid, found := objectToSymbol[named.Obj()]; found {
								fmt.Printf("[DEBUG FUNCTIONS] Setting method's ParentID to existing type symbol with ID=%d\n", pid)
								symbol.ParentID = pid
							} else {
								fmt.Printf("[DEBUG FUNCTIONS] Parent type symbol not found, creating external stub for: %s\n", named.Obj().Name())
								// Create an external stub if the parent type doesn't exist yet
								newID, success := getOrCreateStubForExternal(
									named.Obj(),
									objectToSymbol,
									&symbols,
									&currentID,
								)
								if success {
									fmt.Printf("[DEBUG FUNCTIONS] External stub created with ID=%d, setting as parent\n", newID)
									symbol.ParentID = newID
								}
							}
							// Mark the symbol as a method
							symbol.Kind = SymbolKindMethod
						} else {
							fmt.Printf("[DEBUG FUNCTIONS] It's a function with an unrecognized receiver type\n")
							// It's just a function with a weird receiver we can't resolve
						}
					}
				}
			}

			// Special case for imports (PkgName)
			if pkgNameObj, ok := obj.(*types.PkgName); ok {
				symbol.PackagePath = pkgNameObj.Imported().Path()
				symbol.Name = pkgNameObj.Imported().Path()
				// Record import reference from the current package to this import symbol
				importRefs = append(importRefs, Reference{
					FromID:  pkgSymID,
					ToID:    symbol.ID,
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

	// ------------------------------------------------------------
	// Pass 3: refine type symbols (struct/interface), add fields & interface methods.
	// We'll read the existing "type" symbols from objectToSymbol, see if their
	// underlying type is struct or interface, and update them accordingly.

	// Build a quick reverse map from symbol ID to types.Object
	idToObject := make(map[int]types.Object, len(objectToSymbol))
	for obj, sid := range objectToSymbol {
		idToObject[sid] = obj
	}

	// Index to find symbol by ID quickly
	symbolIndexMap := make(map[int]int, len(symbols))
	for i, s := range symbols {
		symbolIndexMap[s.ID] = i
	}

	for _, s := range symbols {
		if s.Kind == SymbolKindType {
			obj := idToObject[s.ID]
			if typeName, ok := obj.(*types.TypeName); ok {
				under := typeName.Type().Underlying()

				if st, ok := under.(*types.Struct); ok {
					fmt.Printf("[DEBUG FUNCTIONS] Type %s is a struct, refining symbol ID %d\n", typeName.Name(), s.ID)
					// Mark the symbol as struct
					symbols[symbolIndexMap[s.ID]].Kind = SymbolKindStruct

					// Ensure all fields (including embedded) get a symbol
					for i := 0; i < st.NumFields(); i++ {
						fieldObj := st.Field(i)
						fmt.Printf("[DEBUG FUNCTIONS] Checking field: %s in struct: %s\n", fieldObj.Name(), typeName.Name())
						if fieldID, found := objectToSymbol[fieldObj]; found {
							// We already have a symbol for this field
							fieldIdx := symbolIndexMap[fieldID]
							symbols[fieldIdx].Kind = SymbolKindField
							symbols[fieldIdx].Receiver = typeName.Type().String()
							symbols[fieldIdx].ParentID = s.ID
							fmt.Printf("[DEBUG FUNCTIONS] Field symbol found in objectToSymbol, ID=%d\n", fieldID)
						} else {
							// Create a new field symbol
							currentID++
							newField := Symbol{
								ID:          currentID,
								Name:        fieldObj.Name(),
								Kind:        SymbolKindField,
								PackagePath: s.PackagePath,
								Receiver:    typeName.Type().String(),
								ParentID:    s.ID,
							}
							symbols = append(symbols, newField)
							objectToSymbol[fieldObj] = currentID
							symbolIndexMap[currentID] = len(symbols) - 1
							fmt.Printf("[DEBUG FUNCTIONS] Created new field symbol with ID=%d for field: %s\n", currentID, fieldObj.Name())
						}
					}
				} else if iface, ok := under.(*types.Interface); ok {
					// Mark the symbol as interface
					symbols[symbolIndexMap[s.ID]].Kind = SymbolKindInterface
					symbols[symbolIndexMap[s.ID]].Sig = buildInterfaceSignature(typeName, iface)
					fmt.Printf("[DEBUG FUNCTIONS] Interface refined: %s now marked as interface with signature.\n", s.Name)

					// For each interface method, create a child symbol if needed
					for i := 0; i < iface.NumMethods(); i++ {
						m := iface.Method(i)
						fmt.Printf("[DEBUG FUNCTIONS] Checking interface method: %s\n", m.Name())
						if _, found := objectToSymbol[m]; found {
							fmt.Printf("[DEBUG FUNCTIONS] Symbol for interface method: %s already exists, skipping.\n", m.Name())
							// Symbol already exists for this method
							continue
						}
						currentID++
						methodSig := ""
						if msig, ok := m.Type().(*types.Signature); ok {
							methodSig = buildFuncSignature(m, msig)
						}
						methodSym := Symbol{
							ID:          currentID,
							Name:        m.Name(),
							Kind:        SymbolKindMethod,
							PackagePath: s.PackagePath,
							File:        s.File, // Not precise, but we reuse
							Line:        s.Line,
							Column:      s.Column,
							Receiver:    s.Name, // e.g. "MyInterface"
							Sig:         methodSig,
							External:    s.External,
							ParentID:    s.ID,
						}
						symbols = append(symbols, methodSym)
						objectToSymbol[m] = currentID
						symbolIndexMap[currentID] = len(symbols) - 1
					}
				}
				// else: remain SymbolKindType for alias or other named types
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
	// Build a reverse lookup from symbol ID to types.Object
	idToObject := make(map[int]types.Object, len(objectToSymbol))
	for obj, sid := range objectToSymbol {
		idToObject[sid] = obj
	}

	// Gather all named concrete types and interfaces
	typeEntries, interfaceEntries := gatherTypeAndInterfaceEntries(symbols, idToObject)

	// 1) Interface implementations
	implRefs := extractInterfaceImplementations(typeEntries, interfaceEntries, objectToSymbol, &symbols, &maxID)
	out = append(out, implRefs...)

	// 2) Struct embeddings
	structEmbedRefs := extractStructEmbeddings(typeEntries, objectToSymbol, &symbols, &maxID)
	out = append(out, structEmbedRefs...)

	// 3) Interface embeddings
	ifaceEmbedRefs := extractInterfaceEmbeddings(interfaceEntries, objectToSymbol, &symbols, &maxID)
	out = append(out, ifaceEmbedRefs...)

	// 4) Field->type usage references
	fieldTypeUsageRefs := extractFieldTypeUsageReferences(symbols, idToObject, objectToSymbol, &symbols, &maxID)
	out = append(out, fieldTypeUsageRefs...)

	// 5) Func->param/return type usage references
	funcParamReturnTypeRefs := extractFuncParamReturnTypeReferences(symbols, idToObject, objectToSymbol, &symbols, &maxID)
	out = append(out, funcParamReturnTypeRefs...)

	fmt.Printf("\nType relations summary:\n")
	countImplements := countRefsByType(implRefs, "implements")
	countStructEmbeds := countRefsByType(structEmbedRefs, "embeds")
	countInterfaceEmbeds := countRefsByType(ifaceEmbedRefs, "embeds")
	countTypeUsage := len(fieldTypeUsageRefs) + len(funcParamReturnTypeRefs)
	fmt.Printf("  - Interface implementations: %d\n", countImplements)
	fmt.Printf("  - Struct embeddings: %d\n", countStructEmbeds)
	fmt.Printf("  - Interface embeddings: %d\n", countInterfaceEmbeds)
	fmt.Printf("  - Type usage references: %d\n", countTypeUsage)
	fmt.Printf("  - Total type relations: %d\n", len(out))

	return out
}

// gatherTypeAndInterfaceEntries scans the provided symbols and uses idToObject
// to determine if a symbol is a named type (struct) or an interface.
func gatherTypeAndInterfaceEntries(
	symbols []Symbol,
	idToObject map[int]types.Object,
) (
	[]namedTypeEntry,
	[]namedInterfaceEntry,
) {
	var typeEntries []namedTypeEntry
	var interfaceEntries []namedInterfaceEntry

	for _, sym := range symbols {
		if sym.Kind != SymbolKindType && sym.Kind != SymbolKindStruct && sym.Kind != SymbolKindInterface {
			continue
		}
		obj := idToObject[sym.ID]
		typeName, ok := obj.(*types.TypeName)
		if !ok || typeName == nil {
			continue
		}
		typ := typeName.Type()

		if iface, ok := typ.Underlying().(*types.Interface); ok {
			// Build a set of method names for this interface
			methodSet := make(map[string]bool)
			for i := 0; i < iface.NumMethods(); i++ {
				method := iface.Method(i)
				methodSet[method.Name()] = true
			}
			interfaceEntries = append(interfaceEntries, namedInterfaceEntry{
				id:        sym.ID,
				obj:       obj,
				ifaceType: iface,
				name:      typeName.Name(),
				pkgPath:   sym.PackagePath,
				methodSet: methodSet,
			})
		} else {
			_, isStruct := typ.Underlying().(*types.Struct)
			typeEntries = append(typeEntries, namedTypeEntry{
				id:       sym.ID,
				obj:      obj,
				typ:      typ,
				name:     typeName.Name(),
				pkgPath:  sym.PackagePath,
				isStruct: isStruct,
			})
		}
	}
	return typeEntries, interfaceEntries
}

// namedTypeEntry holds info for a single named type (not an interface).
type namedTypeEntry struct {
	id       int
	obj      types.Object
	typ      types.Type
	name     string
	pkgPath  string
	isStruct bool
}

// namedInterfaceEntry holds info for a single named interface.
type namedInterfaceEntry struct {
	id        int
	obj       types.Object
	ifaceType *types.Interface
	name      string
	pkgPath   string
	methodSet map[string]bool
}

// extractInterfaceImplementations checks which concrete types implement which interfaces.
func extractInterfaceImplementations(
	typeEntries []namedTypeEntry,
	interfaceEntries []namedInterfaceEntry,
	objectToSymbol map[types.Object]int,
	symbols *[]Symbol,
	maxID *int,
) []Reference {
	var out []Reference

	fmt.Printf("Checking interface implementations (%d interfaces, %d concrete types)...\n",
		len(interfaceEntries), len(typeEntries))

	for _, ifaceE := range interfaceEntries {
		ifaceType := ifaceE.ifaceType
		for _, conc := range typeEntries {
			// Check if the type implements the interface directly or via pointer
			if types.Implements(conc.typ, ifaceType) || types.Implements(types.NewPointer(conc.typ), ifaceType) {
				r := Reference{
					FromID:  conc.id,
					ToID:    ifaceE.id,
					RefType: "implements",
				}
				out = append(out, r)
				fmt.Printf("  Detected: %s.%s implements %s.%s\n",
					conc.pkgPath, conc.name, ifaceE.pkgPath, ifaceE.name)
			}
		}
	}
	return out
}

// extractStructEmbeddings checks for embedded fields in struct types
func extractStructEmbeddings(
	typeEntries []namedTypeEntry,
	objectToSymbol map[types.Object]int,
	symbols *[]Symbol,
	maxID *int,
) []Reference {
	var out []Reference
	fmt.Println("Checking struct embeddings...")

	for _, conc := range typeEntries {
		if !conc.isStruct {
			continue
		}
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
					embedName := embedObj.Name()
					embedPkg := ""
					if embedObj.Pkg() != nil {
						embedPkg = embedObj.Pkg().Path()
					}

					embedID, found := objectToSymbol[embedObj]
					if !found {
						newID, success := getOrCreateStubForExternal(embedObj, objectToSymbol, symbols, maxID)
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
					fmt.Printf("  Detected: %s.%s embeds %s.%s\n",
						conc.pkgPath, conc.name, embedPkg, embedName)
				}
			}
		}
	}
	return out
}

// extractInterfaceEmbeddings checks for embedded interfaces in other interfaces
func extractInterfaceEmbeddings(
	interfaceEntries []namedInterfaceEntry,
	objectToSymbol map[types.Object]int,
	symbols *[]Symbol,
	maxID *int,
) []Reference {
	var out []Reference
	fmt.Println("Checking interface embeddings...")

	for _, ifaceE := range interfaceEntries {
		ifa := ifaceE.ifaceType
		for i := 0; i < ifa.NumEmbeddeds(); i++ {
			embeddedT := ifa.EmbeddedType(i)
			if named, ok := embeddedT.(*types.Named); ok {
				if _, ok2 := named.Underlying().(*types.Interface); ok2 {
					embedObj := named.Obj()
					embedName := embedObj.Name()
					embedPkg := ""
					if embedObj.Pkg() != nil {
						embedPkg = embedObj.Pkg().Path()
					}

					embedID, found := objectToSymbol[embedObj]
					if !found {
						newID, success := getOrCreateStubForExternal(embedObj, objectToSymbol, symbols, maxID)
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
					fmt.Printf("  Detected: interface %s.%s embeds interface %s.%s\n",
						ifaceE.pkgPath, ifaceE.name, embedPkg, embedName)
				}
			}
		}
	}
	return out
}

// extractFieldTypeUsageReferences creates "usage" references from each field symbol to its type (if named).
func extractFieldTypeUsageReferences(
	symbols []Symbol,
	idToObject map[int]types.Object,
	objectToSymbol map[types.Object]int,
	symbolsPtr *[]Symbol,
	maxID *int,
) []Reference {

	var out []Reference
	fmt.Println("Processing field-to-type usage references...")

	fieldCount := 0
	processedCount := 0

	for _, sym := range symbols {
		if sym.Kind != SymbolKindField {
			continue
		}

		fieldCount++
		fieldObj := idToObject[sym.ID]
		if fieldObj == nil {
			if VerboseLogging {
				fmt.Printf("[DEBUG FIELD] Field symbol ID %d with name %s has no corresponding object\n",
					sym.ID, sym.Name)
			}
			continue
		}

		if fieldVar, ok := fieldObj.(*types.Var); ok {
			processedCount++
			fieldType := fieldVar.Type()

			// Get parent struct info for better debugging
			parentName := "unknown"
			if sym.ParentID > 0 && VerboseLogging {
				for _, parentSym := range symbols {
					if parentSym.ID == sym.ParentID {
						parentName = parentSym.Name
						break
					}
				}
			}

			if VerboseLogging {
				fmt.Printf("[DEBUG FIELD] Processing field %s.%s of type %s\n",
					parentName, sym.Name, fieldType.String())
			}

			prevLen := len(out)
			out = addTypeUsageRef(sym.ID, fieldType, out, objectToSymbol, symbolsPtr, maxID)

			if VerboseLogging {
				if len(out) > prevLen {
					fmt.Printf("[DEBUG FIELD] Added %d type usage reference(s) for field %s\n",
						len(out)-prevLen, sym.Name)
				} else {
					fmt.Printf("[DEBUG FIELD WARNING] No type usage references added for field %s of type %s\n",
						sym.Name, fieldType.String())

					// Special handling for external types that might be problematic
					// First attempt to explicitly create a stub for this type
					if EnableEnhancedExternalTypes {
						if named, isNamed := fieldType.(*types.Named); isNamed && named.Obj().Pkg() != nil {
							fmt.Printf("[DEBUG FIELD] Attempting special handling for named external type %s.%s\n",
								named.Obj().Pkg().Path(), named.Obj().Name())

							if _, found := objectToSymbol[named.Obj()]; !found {
								newID, success := getOrCreateStubForExternal(named.Obj(), objectToSymbol, symbolsPtr, maxID)
								if success {
									out = append(out, Reference{
										FromID:  sym.ID,
										ToID:    newID,
										RefType: "usage",
									})
									fmt.Printf("[DEBUG FIELD] Successfully added special usage reference from field %s to type %s\n",
										sym.Name, named.Obj().Name())
								}
							}
						} else if ptr, isPtr := fieldType.(*types.Pointer); isPtr {
							if named, isNamed := ptr.Elem().(*types.Named); isNamed && named.Obj().Pkg() != nil {
								fmt.Printf("[DEBUG FIELD] Attempting special handling for pointer to named external type %s.%s\n",
									named.Obj().Pkg().Path(), named.Obj().Name())

								if _, found := objectToSymbol[named.Obj()]; !found {
									newID, success := getOrCreateStubForExternal(named.Obj(), objectToSymbol, symbolsPtr, maxID)
									if success {
										out = append(out, Reference{
											FromID:  sym.ID,
											ToID:    newID,
											RefType: "usage",
										})
										fmt.Printf("[DEBUG FIELD] Successfully added special usage reference from field %s to pointer type %s\n",
											sym.Name, named.Obj().Name())
									}
								}
							}
						}
					}
				}
			}
		} else if VerboseLogging {
			fmt.Printf("[DEBUG FIELD WARNING] Field symbol ID %d with name %s has non-var object type %T\n",
				sym.ID, sym.Name, fieldObj)
		}
	}

	if VerboseLogging {
		fmt.Printf("[DEBUG FIELD SUMMARY] Processed %d/%d field symbols, created %d usage references\n",
			processedCount, fieldCount, len(out))
	}

	return out
}

// extractFuncParamReturnTypeReferences creates usage references from each func/method to its parameter and return types.
func extractFuncParamReturnTypeReferences(
	symbols []Symbol,
	idToObject map[int]types.Object,
	objectToSymbol map[types.Object]int,
	symbolsPtr *[]Symbol,
	maxID *int,
) []Reference {

	var out []Reference
	fmt.Println("Processing function parameter and return type references...")

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
			out = addTypeUsageRef(sym.ID, paramType, out, objectToSymbol, symbolsPtr, maxID)
		}
		// Results
		for i := 0; i < sig.Results().Len(); i++ {
			resultType := sig.Results().At(i).Type()
			out = addTypeUsageRef(sym.ID, resultType, out, objectToSymbol, symbolsPtr, maxID)
		}
	}
	return out
}

// addTypeUsageRef is a helper that adds a "usage" reference from fromSid to t, if named. Recur for pointers, slices, etc.
func addTypeUsageRef(
	fromSid int,
	t types.Type,
	out []Reference,
	objectToSymbol map[types.Object]int,
	symbolsPtr *[]Symbol,
	maxID *int,
) []Reference {
	return addTypeUsageRefWithDepth(fromSid, t, out, objectToSymbol, symbolsPtr, maxID, 0)
}

// addTypeUsageRefWithDepth is the internal implementation of addTypeUsageRef with a recursion depth counter
func addTypeUsageRefWithDepth(
	fromSid int,
	t types.Type,
	out []Reference,
	objectToSymbol map[types.Object]int,
	symbolsPtr *[]Symbol,
	maxID *int,
	depth int,
) []Reference {
	// Prevent stack overflows with deep recursion
	const maxRecursionDepth = 5
	if depth > maxRecursionDepth {
		if VerboseLogging {
			fmt.Printf("[DEBUG TYPE USAGE] Recursion depth limit reached (%d) for type %s\n",
				depth, t.String())
		}
		return out
	}

	if out == nil {
		out = []Reference{} // Initialize if nil
	}

	switch tt := t.(type) {
	case *types.Pointer:
		return addTypeUsageRefWithDepth(fromSid, tt.Elem(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
	case *types.Named:
		if typeSid, found := objectToSymbol[tt.Obj()]; found {
			out = append(out, Reference{
				FromID:  fromSid,
				ToID:    typeSid,
				RefType: "usage",
			})
			if VerboseLogging {
				fmt.Printf("[DEBUG TYPE USAGE] Created usage reference from %d to existing type %s (ID: %d)\n",
					fromSid, tt.Obj().Name(), typeSid)
			}
		} else {
			// Possibly create an external stub
			newID, success := getOrCreateStubForExternal(tt.Obj(), objectToSymbol, symbolsPtr, maxID)
			if success {
				out = append(out, Reference{
					FromID:  fromSid,
					ToID:    newID,
					RefType: "usage",
				})
				if VerboseLogging {
					fmt.Printf("[DEBUG TYPE USAGE] Created usage reference from %d to new external type %s (ID: %d)\n",
						fromSid, tt.Obj().Name(), newID)
				}
			} else if VerboseLogging {
				// Log when we fail to create a reference to help with debugging
				pkg := "unknown"
				if tt.Obj().Pkg() != nil {
					pkg = tt.Obj().Pkg().Path()
				}
				fmt.Printf("[DEBUG TYPE USAGE WARNING] Failed to create stub for external type %s.%s\n",
					pkg, tt.Obj().Name())
			}
		}
		// For named composite types (structs with fields, etc.), we should also process the underlying type
		// to ensure we capture all possible relationships
		_, isStruct := tt.Underlying().(*types.Struct)
		_, isInterface := tt.Underlying().(*types.Interface)
		if EnableEnhancedExternalTypes && (isStruct || isInterface) && depth < maxRecursionDepth-1 {
			out = addTypeUsageRefForUnderlyingWithDepth(fromSid, tt.Underlying(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
		}
	case *types.Slice:
		return addTypeUsageRefWithDepth(fromSid, tt.Elem(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
	case *types.Array:
		return addTypeUsageRefWithDepth(fromSid, tt.Elem(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
	case *types.Chan:
		return addTypeUsageRefWithDepth(fromSid, tt.Elem(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
	case *types.Map:
		out = addTypeUsageRefWithDepth(fromSid, tt.Key(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
		return addTypeUsageRefWithDepth(fromSid, tt.Elem(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
	case *types.Struct:
		// For anonymous structs, we should still process their fields
		if EnableEnhancedExternalTypes && depth < maxRecursionDepth-1 {
			return addTypeUsageRefForUnderlyingWithDepth(fromSid, tt, out, objectToSymbol, symbolsPtr, maxID, depth+1)
		}
	case *types.Interface:
		// For anonymous interfaces, we should still process their methods
		if EnableEnhancedExternalTypes && depth < maxRecursionDepth-1 {
			return addTypeUsageRefForUnderlyingWithDepth(fromSid, tt, out, objectToSymbol, symbolsPtr, maxID, depth+1)
		}
	}
	return out
}

// addTypeUsageRefForUnderlying adds type references for fields of structs or methods of interfaces
func addTypeUsageRefForUnderlying(
	fromSid int,
	t types.Type,
	out []Reference,
	objectToSymbol map[types.Object]int,
	symbolsPtr *[]Symbol,
	maxID *int,
) []Reference {
	return addTypeUsageRefForUnderlyingWithDepth(fromSid, t, out, objectToSymbol, symbolsPtr, maxID, 0)
}

// addTypeUsageRefForUnderlyingWithDepth is the internal implementation with recursion depth counter
func addTypeUsageRefForUnderlyingWithDepth(
	fromSid int,
	t types.Type,
	out []Reference,
	objectToSymbol map[types.Object]int,
	symbolsPtr *[]Symbol,
	maxID *int,
	depth int,
) []Reference {
	// Prevent stack overflows with deep recursion
	const maxRecursionDepth = 5
	if depth > maxRecursionDepth {
		if VerboseLogging {
			fmt.Printf("[DEBUG TYPE USAGE] Recursion depth limit reached (%d) for underlying type %s\n",
				depth, t.String())
		}
		return out
	}

	if out == nil {
		out = []Reference{} // Initialize if nil
	}

	switch tt := t.(type) {
	case *types.Struct:
		// Add references for all field types
		for i := 0; i < tt.NumFields(); i++ {
			field := tt.Field(i)
			fieldType := field.Type()
			out = addTypeUsageRefWithDepth(fromSid, fieldType, out, objectToSymbol, symbolsPtr, maxID, depth+1)
		}
	case *types.Interface:
		// Add references for all method signature types
		for i := 0; i < tt.NumMethods(); i++ {
			method := tt.Method(i)
			sig, ok := method.Type().(*types.Signature)
			if !ok {
				continue
			}

			// Add references for param types
			if params := sig.Params(); params != nil {
				for j := 0; j < params.Len(); j++ {
					out = addTypeUsageRefWithDepth(fromSid, params.At(j).Type(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
				}
			}

			// Add references for return types
			if results := sig.Results(); results != nil {
				for j := 0; j < results.Len(); j++ {
					out = addTypeUsageRefWithDepth(fromSid, results.At(j).Type(), out, objectToSymbol, symbolsPtr, maxID, depth+1)
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

// countRefsByType returns how many references have the given refType.
func countRefsByType(refs []Reference, refType string) int {
	count := 0
	for _, r := range refs {
		if r.RefType == refType {
			count++
		}
	}
	return count
}
