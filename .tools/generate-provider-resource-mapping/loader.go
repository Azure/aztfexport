package main

import (
	"errors"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type Package struct {
	GoPackage  *packages.Package
	SSAPackage *ssa.Package
	CallGraph  *callgraph.Graph
}

func loadPackage(dir string, args []string) ([]*Package, error) {
	var pkgs []*Package
	cfg := packages.Config{Dir: dir, Mode: packages.LoadAllSyntax}
	gopkgs, err := packages.Load(&cfg, args...)
	if err != nil {
		return nil, err
	}

	if packages.PrintErrors(gopkgs) > 0 {
		return nil, errors.New("go packages contain errors during loading")
	}

	prog, ssapkgs := ssautil.Packages(gopkgs, 0)
	for _, p := range ssapkgs {
		if p != nil {
			p.Build()
		}
	}
	// CHA is a good fit here since we are not building the SSA bodies for dependencies (ssautil.Packages).
	// CHA is sound to run on partial program (i.e. no main package is required). In our case, we are using the CHA
	// callgraph to find any callee of a provider resource method (typically the "Delete" method) which belong to
	// the corresponding function from the Go SDK, but not necessarily need to follow the callee outside the package
	// under processed.
	graph := cha.CallGraph(prog)

	for idx := range ssapkgs {
		pkgs = append(pkgs,
			&Package{
				GoPackage:  gopkgs[idx],
				SSAPackage: ssapkgs[idx],
				CallGraph:  graph,
			},
		)
	}

	return pkgs, nil
}
