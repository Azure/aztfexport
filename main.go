package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"flag"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/ui"
)

var (
	flagVersion     *bool
	flagOutputDir   *string
	flagMappingFile *string
	flagContinue    *bool
)

func init() {
	flagVersion = flag.Bool("v", false, "Print version")
	flagOutputDir = flag.String("o", "", "Specify output dir. Default is a dir under the user cache dir, which is named after the resource group name")
	flagMappingFile = flag.String("m", "", "Specify the resource mapping file (for batch import)")
	flagContinue = flag.Bool("k", false, "Whether continue on import error (batch import only)")
}

const usage = `aztfy [option] <resource group name>
`

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
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

	rg := flag.Args()[0]

	cfg, err := config.NewConfig(rg, *flagOutputDir)
	if err != nil {
		fatal(err)
	}

	if *flagMappingFile != "" {
		if err := batchImport(*cfg, *flagMappingFile, *flagContinue); err != nil {
			fatal(err)
		}
		return
	}

	prog, err := ui.NewProgram(*cfg)
	if err != nil {
		fatal(err)
	}

	if err := prog.Start(); err != nil {
		fatal(err)
	}
}

func batchImport(cfg config.Config, mappingFile string, continueOnError bool) error {
	b, err := os.ReadFile(mappingFile)
	if err != nil {
		return fmt.Errorf("reading mapping file %s: %v", mappingFile, err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("unmarshalling the mapping file: %v", err)
	}

	// Discard logs from hashicorp/azure-go-helper
	log.SetOutput(io.Discard)
	// Define another dedicated logger for the ui
	logger := log.New(os.Stderr, "", log.LstdFlags)
	if cfg.Logfile != "" {
		f, err := os.OpenFile(cfg.Logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		logger = log.New(f, "aztfy", log.LstdFlags)
	}

	logger.Println("New meta")
	c, err := meta.NewMeta(cfg)
	if err != nil {
		return err
	}

	logger.Println("Initialize")
	if err := c.Init(); err != nil {
		return err
	}

	logger.Println("List resources")
	list := c.ListResource()

	logger.Println("Import resources")
	for i := range list {
		rt, ok := m[list[i].ResourceID]
		if !ok {
			logger.Printf("[WARN] No mapping information for resource: %s, skip it\n", list[i].ResourceID)
			continue
		}
		list[i].TFResourceType = rt
		logger.Printf("Importing %s as %s\n", list[i].ResourceID, list[i].TFAddr())
		c.Import(&list[i])
		if err := list[i].ImportError; err != nil {
			msg := fmt.Sprintf("Failed to import %s as %s: %v", list[i].ResourceID, list[i].TFAddr(), err)
			if !continueOnError {
				return fmt.Errorf(msg)
			}
			logger.Println("[ERROR] " + msg)
		}
	}

	logger.Println("Generate Terraform configurations")
	if err := c.GenerateCfg(list); err != nil {
		return fmt.Errorf("generating Terraform configuration: %v", err)
	}

	return nil
}
