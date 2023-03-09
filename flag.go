package main

import (
	"github.com/urfave/cli/v2"
)

var flagset FlagSet

type FlagSet struct {
	// common flags
	flagSubscriptionId      string
	flagOutputDir           string
	flagOverwrite           bool
	flagAppend              bool
	flagDevProvider         bool
	flagBackendType         string
	flagBackendConfig       cli.StringSlice
	flagFullConfig          bool
	flagParallelism         int
	flagContinue            bool
	flagNonInteractive      bool
	flagGenerateMappingFile bool
	flagHCLOnly             bool
	flagModulePath          string

	// common flags (hidden)
	hflagMockClient bool
	hflagPlainUI    bool
	hflagProfile    string

	// Subcommand specific flags
	//
	// res:
	// flagResName
	// flagResType
	//
	// rg:
	// flagPattern
	//
	// query:
	// flagPattern
	// flagRecursive
	flagPattern   string
	flagRecursive bool
	flagResName   string
	flagResType   string
}
