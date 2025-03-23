package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	symbols, objectToSymbol, packageToSymbol := ExtractSymbols(pkgs)

	fmt.Printf("\nParsed %d package(s)\n", len(pkgs))
	fmt.Printf("Extracted %d symbol(s):\n", len(symbols))
	for _, s := range symbols {
		fmt.Printf("- ID: %d, Kind: %s, Name: %s, Receiver: %s, File: %s, Line: %d, Col: %d\n",
			s.ID, s.Kind, s.Name, s.Receiver, s.File, s.Line, s.Column)
	}

	// Extract references (usages) among the symbols
	references := ExtractReferences(pkgs, objectToSymbol, packageToSymbol)

	fmt.Printf("\nExtracted %d reference(s):\n", len(references))
	for _, r := range references {
		fmt.Printf("- from: %d to: %d, file: %s, line: %d, col: %d, type: %s\n",
			r.FromID, r.ToID, r.File, r.Line, r.Column, r.RefType)
	}

	// Now detect interface implementations and embedded structs:
	typeRelations := ExtractTypeRelations(pkgs, symbols, objectToSymbol, packageToSymbol)
	references = append(references, typeRelations...)

	fmt.Printf("\nExtracted %d additional type-relations:\n", len(typeRelations))
	for _, r := range typeRelations {
		fmt.Printf("- from: %d to: %d, relationship: %s\n", r.FromID, r.ToID, r.RefType)
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

	if err := ExportToJSON(jsonOutput, symbols, references); err != nil {
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

	fmt.Printf("Sourcetrail DB created at: %s\n", dbOutput)

	// Optional: remove JSON after success
	if !keepJSON {
		if err := os.Remove(jsonOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: couldn't remove intermediate JSON file: %v\n", err)
		}
	}

	fmt.Println("Done.")
}