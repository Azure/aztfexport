package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/ui/common"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/magodo/spinner"
)

func BatchImport(cfg config.Config) error {
	c, err := meta.NewMeta(cfg)
	if err != nil {
		return err
	}

	var errors []string

	f := func(msg Messager) error {
		msg.SetStatus("Initializing...")
		if err := c.Init(); err != nil {
			return err
		}

		msg.SetStatus("Listing resources...")
		list, err := c.ListResource()
		if err != nil {
			return err
		}

		msg.SetStatus("Exporting Skipped Resource file...")
		if err := c.ExportSkippedResources(list); err != nil {
			return fmt.Errorf("exporting Skipped Resource file: %v", err)
		}

		msg.SetStatus("Exporting Resource Mapping file...")
		if err := c.ExportResourceMapping(list); err != nil {
			return fmt.Errorf("exporting Resource Mapping file: %v", err)
		}

		// Return early if only generating mapping file
		if cfg.GenerateMappingFile {
			return nil
		}

		msg.SetStatus("Importing resources...")
		for i := range list {
			if list[i].Skip() {
				continue
			}
			msg.SetStatus(fmt.Sprintf("(%d/%d) Importing %s as %s", i+1, len(list), list[i].TFResourceId, list[i].TFAddr))
			c.Import(&list[i])
			if err := list[i].ImportError; err != nil {
				msg := fmt.Sprintf("Failed to import %s as %s: %v", list[i].TFResourceId, list[i].TFAddr, err)
				if !cfg.ContinueOnError {
					return fmt.Errorf(msg)
				}
				errors = append(errors, msg)
			}
		}

		msg.SetStatus("Generating Terraform configurations...")
		if err := c.GenerateCfg(list); err != nil {
			return fmt.Errorf("generating Terraform configuration: %v", err)
		}

		return nil
	}

	if cfg.PlainUI {
		err = f(&StdoutMessager{})
	} else {
		s := bspinner.NewModel()
		s.Spinner = common.Spinner
		sf := func(msg spinner.Messager) error {
			return f(&msg)
		}
		err = spinner.Run(s, sf)
	}

	if err != nil {
		return err
	}

	// Print out the errors, if any
	if len(errors) != 0 {
		fmt.Fprintln(os.Stderr, "Errors:\n"+strings.Join(errors, "\n"))
	}

	return nil
}
