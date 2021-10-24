package main

import (
	"errors"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type Package struct {
	GoPackage  *packages.Package
	SSAPackage *ssa.Package
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

	_, ssapkgs := ssautil.Packages(gopkgs, 0)
	for _, p := range ssapkgs {
		if p != nil {
			p.Build()
		}
	}

	for idx := range ssapkgs {
		pkgs = append(pkgs,
			&Package{
				GoPackage:  gopkgs[idx],
				SSAPackage: ssapkgs[idx],
			},
		)
	}

	return pkgs, nil
}
