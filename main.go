package main

import (
	"context"
	"fmt"
	"os"

	"flag"

	"github.com/magodo/aztfy/internal"
	"github.com/magodo/aztfy/internal/version"
)

var (
	flagVersion *bool
)

func init() {
	flagVersion = flag.Bool("v", false, "print version")
}

const usage = `aztfy [option] <resource group name>
`

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%s\n", usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *flagVersion {
		fmt.Println(version.GetVersion())
		os.Exit(0)
	}

	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	if err := internal.Run(context.TODO(), flag.Args()[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
