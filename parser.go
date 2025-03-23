package main

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

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