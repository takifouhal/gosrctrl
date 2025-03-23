package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var pathArg string
	var outArg string

	flag.StringVar(&pathArg, "path", ".", "Path to the Go project (default: current directory)")
	flag.StringVar(&outArg, "out", "output.srctrldb", "Output file for Sourcetrail DB (default: output.srctrldb)")

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
	fmt.Println("\n(Parsing logic to be implemented in future tasks.)")
}