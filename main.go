package main

import (
	"fmt"
	"log"
	"os"

	"flag"

	"github.com/magodo/aztfy/internal/config"
	"github.com/magodo/aztfy/internal/ui"
)

var (
	flagVersion *bool
)

func init() {
	flagVersion = flag.Bool("v", false, "Print version")
}

// var reader = bufio.NewReader(os.Stdin)

const usage = `aztfy [option] <resource group name>
`

func main() {
	// os.Setenv("AZTFY_DEBUG", "true")
	// os.Setenv("AZTFY_MOCK_CLIENT", "false")
	// os.Setenv("AZTFY_LOGFILE", "aztfylogs.log")

	// log.Println("Main hitted")

	// input, _ := reader.ReadString('\n')
	// value := strings.TrimSpace(input)
	// fmt.Printf("input: %v", value)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%s\n", usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *flagVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := config.NewConfig(flag.Args()[0])
	if err != nil {
		log.Fatal(err)
	}

	prog, err := ui.NewProgram(*cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := prog.Start(); err != nil {
		log.Fatal(err)
	}
}
