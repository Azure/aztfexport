package main

import (
	"context"
	"fmt"
	"os"

	"github.com/magodo/aztfy/internal"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, `Usage: aztfy <resource_group_name>`)
		os.Exit(1)
	}

	if err := internal.Run(context.TODO(), os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
