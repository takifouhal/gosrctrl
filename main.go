package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

func main() {
	var pathArg string
	var outArg string
	var keepJSON bool

	flag.StringVar(&pathArg, "path", ".", "Path to the Go project (default: current directory)")
	flag.StringVar(&outArg, "out", "output.srctrldb", "Output file for Sourcetrail DB (default: output.srctrldb)")
	flag.BoolVar(&keepJSON, "keepjson", false, "Keep intermediate JSON file (default: false)")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"GoSrcCtrl - Go Source Code Parser and Indexer\n\n"+
				"Usage: %s [options]\n\n"+
				"Options:\n",
			filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}

	flag.Parse()

	// If help was invoked or no flags provided, display usage
	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(0)
	}

	// Check if the specified path exists and is a directory
	info, err := os.Stat(pathArg)
	if os.IsNotExist(err) {
		fmt.Printf("Error: specified path does not exist: %s\n", pathArg)
		os.Exit(1)
	} else if err != nil {
		fmt.Printf("Error checking path: %v\n", err)
		os.Exit(1)
	} else if !info.IsDir() {
		fmt.Printf("Error: specified path is not a directory: %s\n", pathArg)
		os.Exit(1)
	}

	fmt.Println("GoSrcCtrl - Go Source Code Parser and Indexer")
	fmt.Println("---------------------------------------------")
	fmt.Printf("Project path: %s\n", pathArg)
	fmt.Printf("Output file : %s\n", outArg)

	// Load Go packages to parse
	pkgs, loadErr := LoadPackages(pathArg)
	if loadErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", loadErr)
		os.Exit(1)
	}

	// Extract symbols (definitions) and build object->symbol map + package->symbol map
	symbols, importRefs, objectToSymbol, packageToSymbol := ExtractSymbols(pkgs)

	fmt.Printf("\nParsed %d package(s)\n", len(pkgs))
	fmt.Printf("Extracted %d symbol(s):\n", len(symbols))
	for _, s := range symbols {
		fmt.Printf("- ID: %d, Kind: %s, Name: %s, Receiver: %s, File: %s, Line: %d, Col: %d\n",
			s.ID, s.Kind, s.Name, s.Receiver, s.File, s.Line, s.Column)
	}

	// Extract references (usages) among the symbols
	references := ExtractReferences(pkgs, symbols, objectToSymbol, packageToSymbol)
	references = append(references, importRefs...)

	fmt.Printf("\nExtracted %d reference(s):\n", len(references))
	for _, r := range references {
		fmt.Printf("- from: %d to: %d, file: %s, line: %d, col: %d, type: %s\n",
			r.FromID, r.ToID, r.File, r.Line, r.Column, r.RefType)
	}

	// Now detect interface implementations and embedded structs:
	typeRelations := ExtractTypeRelations(pkgs, symbols, objectToSymbol, packageToSymbol)
	references = append(references, typeRelations...)

	fmt.Printf("\nExtracted %d additional type-relations:\n", len(typeRelations))
	
	// Count different types of relationships
	implCount := 0
	embedCount := 0
	usageCount := 0
	
	for _, r := range typeRelations {
		switch r.RefType {
		case "implements":
			implCount++
		case "embeds":
			embedCount++
		case "usage":
			usageCount++
		}
	}
	
	fmt.Printf("- Interface implementations: %d\n", implCount)
	fmt.Printf("- Struct/interface embeddings: %d\n", embedCount)
	fmt.Printf("- Type usage references: %d\n", usageCount)
	
	// Print detailed relationship information for debugging
	if len(typeRelations) <= 20 { // Only show details for smaller projects
		fmt.Println("\nDetailed type relationships:")
		for _, r := range typeRelations {
			fromSym := findSymbolByID(symbols, r.FromID)
			toSym := findSymbolByID(symbols, r.ToID)
			
			fromName := "unknown"
			toName := "unknown"
			
			if fromSym != nil {
				fromName = fmt.Sprintf("%s (%s)", fromSym.Name, fromSym.Kind)
			}
			
			if toSym != nil {
				toName = fmt.Sprintf("%s (%s)", toSym.Name, toSym.Kind)
			}
			
			fmt.Printf("- %s → %s, relationship: %s\n", fromName, toName, r.RefType)
		}
	}

	fmt.Println("\n(References extraction logic can be refined to distinguish calls, reads, etc.)")
	fmt.Println("(Parsing logic to be further developed in future tasks.)")

	// --------------------------------------------------
	// Integration: end-to-end parse + index
	// Decide on final .srctrldb name
	var dbOutput string
	if filepath.Ext(outArg) == ".srctrldb" {
		dbOutput = outArg
	} else {
		dbOutput = outArg + ".srctrldb"
	}

	// Produce a JSON filename
	// If user specifically used .srctrldb as outArg, we produce a separate JSON with same base name
	jsonOutput := strings.TrimSuffix(dbOutput, ".srctrldb") + ".json"

	// Parse go.mod for module info (optional)
	modules, modErr := parseGoMod(pathArg)
	if modErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not parse go.mod: %v\n", modErr)
	}

	if err := ExportToJSON(jsonOutput, symbols, references, modules); err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting to JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nJSON export written to %s\n", jsonOutput)

	// Next: call Python script to generate the .srctrldb
	// Write the embedded Python script to a temporary file
	tmpScript, err := os.CreateTemp("", "generate_db_*.py")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp script file: %v\n", err)
		os.Exit(1)
	}
	tmpScriptPath := tmpScript.Name()
	if _, err := tmpScript.Write(generateDbScript); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to temp script file: %v\n", err)
		os.Exit(1)
	}
	tmpScript.Close()

	// Ensure the output directory exists
	outputDir := filepath.Dir(dbOutput)
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		fmt.Printf("Creating output directory: %s\n", outputDir)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Check if we can write to the output directory
	if err := checkDirectoryWritable(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: output directory is not writable: %v\n", err)
		os.Exit(1)
	}

	generateDBCmd := exec.Command("python3", tmpScriptPath, "-i", jsonOutput, "-o", dbOutput)
	generateDBCmd.Stdout = os.Stdout
	generateDBCmd.Stderr = os.Stderr

	fmt.Printf("\nGenerating Sourcetrail DB using %s...\n", generateDBCmd.String())
	if err := generateDBCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating Sourcetrail DB: %v\n", err)
		os.Exit(1)
	}

	// Clean up the temporary script file
	os.Remove(tmpScriptPath)

	// Verify the DB file was actually created
	if _, err := os.Stat(dbOutput); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Sourcetrail DB file was not created at: %s\n", dbOutput)
		fmt.Fprintf(os.Stderr, "This may indicate an issue with the Python environment or the Numbat library.\n")
		fmt.Fprintf(os.Stderr, "Please check that numbat is installed: pip install numbat==0.2.2\n")
		os.Exit(1)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking Sourcetrail DB file: %v\n", err)
		os.Exit(1)
	} else {
		// Check if the file has content (not zero bytes)
		fileInfo, _ := os.Stat(dbOutput)
		if fileInfo.Size() == 0 {
			fmt.Fprintf(os.Stderr, "Warning: Sourcetrail DB file was created but is empty (0 bytes): %s\n", dbOutput)
		} else {
			fmt.Printf("Sourcetrail DB created at: %s (%.2f KB)\n", dbOutput, float64(fileInfo.Size())/1024.0)
		}
	}

	// Check for the associated .srctrlprj file
	prjFile := strings.TrimSuffix(dbOutput, ".srctrldb") + ".srctrlprj"
	if _, err := os.Stat(prjFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Warning: .srctrlprj project file was not created at: %s\n", prjFile)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking Sourcetrail project file: %v\n", err)
	} else {
		fmt.Printf("Sourcetrail project file created at: %s\n", prjFile)
	}

	// Optional: remove JSON after success
	if !keepJSON {
		if err := os.Remove(jsonOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: couldn't remove intermediate JSON file: %v\n", err)
		}
	}

	fmt.Println("Done.")
}

// parseGoMod tries to read and parse go.mod in the current directory.
// Returns a slice of GoModule or an empty slice if go.mod doesn't exist or can't be parsed.
func parseGoMod(dir string) ([]GoModule, error) {
	modPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(modPath); os.IsNotExist(err) {
		return nil, nil // No go.mod file present
	}

	contents, err := ioutil.ReadFile(modPath)
	if err != nil {
		return nil, err
	}

	parsedFile, err := modfile.Parse("go.mod", contents, nil)
	if err != nil {
		return nil, err
	}

	var modules []GoModule

	// Record the main module, if present
	if parsedFile.Module != nil {
		modules = append(modules, GoModule{
			Path:    parsedFile.Module.Mod.Path,
			Version: "", // main module has no explicit version here, typically
		})
	}

	// For each "require" statement, gather path & version
	for _, r := range parsedFile.Require {
		modules = append(modules, GoModule{
			Path:    r.Mod.Path,
			Version: r.Mod.Version,
		})
	}

	return modules, nil
}

// checkDirectoryWritable tests if the given directory is writable
// by attempting to create a temporary file in it
func checkDirectoryWritable(dirPath string) error {
	// Try to create a temporary file in the directory
	tmpFile, err := os.CreateTemp(dirPath, "write_test_*.tmp")
	if err != nil {
		return err
	}

	// Clean up the temporary file
	tmpFile.Close()
	os.Remove(tmpFile.Name())

	return nil
}

// findSymbolByID returns the symbol with the given ID, or nil if not found
func findSymbolByID(symbols []Symbol, id int) *Symbol {
	for i := range symbols {
		if symbols[i].ID == id {
			return &symbols[i]
		}
	}
	return nil
}