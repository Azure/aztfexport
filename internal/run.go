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

		defer func() {
			msg.SetStatus("DeInitializing...")
			c.DeInit()
		}()

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

		for i := 0; i < len(list); i += cfg.Parallelism {
			n := cfg.Parallelism
			if i+cfg.Parallelism > len(list) {
				n = len(list) - i
			}

			var importList []*meta.ImportItem
			messages := []string{"Importing resources..."}

			for j := 0; j < n; j++ {
				idx := i + j
				if list[idx].Skip() {
					messages = append(messages, fmt.Sprintf("(%d/%d) Skipping %s", idx+1, len(list), list[idx].TFResourceId))
					continue
				}
				messages = append(messages, fmt.Sprintf("(%d/%d) Importing %s as %s", idx+1, len(list), list[idx].TFResourceId, list[idx].TFAddr))
				importList = append(importList, &list[idx])
			}

			msg.SetStatus(strings.Join(messages, "\n"))
			c.ParallelImport(importList)

			var thisErrors []string
			for j := 0; j < n; j++ {
				idx := i + j
				item := list[idx]
				if err := item.ImportError; err != nil {
					msg := fmt.Sprintf("Failed to import %s as %s: %v", item.TFResourceId, item.TFAddr, err)
					thisErrors = append(thisErrors, msg)
				}
			}
			if len(thisErrors) != 0 {
				errors = append(errors, thisErrors...)
				if !cfg.ContinueOnError {
					return fmt.Errorf(strings.Join(thisErrors, "\n"))
				}
			}
		}

		if err := c.PushState(); err != nil {
			return fmt.Errorf("failed to push state: %v", err)
		}

		msg.SetStatus("Generating Terraform configurations...")
		if err := c.GenerateCfg(list); err != nil {
			return fmt.Errorf("generating Terraform configuration: %v", err)
		}

		msg.SetStatus("Cleaning up...")
		if err := c.CleanUpWorkspace(); err != nil {
			return fmt.Errorf("cleaning up main workspace: %v", err)
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
