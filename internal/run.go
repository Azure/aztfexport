package internal

import (
	"context"
	"fmt"
	"os"
	"strings"

	internalmeta "github.com/Azure/aztfexport/internal/meta"

	"github.com/Azure/aztfexport/internal/config"
	"github.com/Azure/aztfexport/pkg/meta"

	"github.com/Azure/aztfexport/internal/ui/common"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/magodo/spinner"
)

func BatchImport(ctx context.Context, cfg config.NonInteractiveModeConfig) error {
	var c meta.Meta = internalmeta.NewGroupMetaDummy(cfg.ResourceGroupName, cfg.ProviderName)
	if !cfg.MockMeta {
		var err error
		c, err = meta.NewMeta(cfg.Config)
		if err != nil {
			return err
		}
	}

	var errors []string

	f := func(msg Messager) error {
		msg.SetStatus("Initializing...")
		if err := c.Init(ctx); err != nil {
			return err
		}

		defer func() {
			msg.SetStatus("DeInitializing...")
			// #nosec G104
			c.DeInit(ctx)
		}()

		msg.SetStatus("Listing resources...")
		list, err := c.ListResource(ctx)
		if err != nil {
			return err
		}

		msg.SetStatus("Exporting Skipped Resource file...")
		if err := c.ExportSkippedResources(ctx, list); err != nil {
			return fmt.Errorf("exporting Skipped Resource file: %v", err)
		}

		msg.SetStatus("Exporting Resource Mapping file...")
		if err := c.ExportResourceMapping(ctx, list); err != nil {
			return fmt.Errorf("exporting Resource Mapping file: %v", err)
		}

		// Return early if only generating mapping file
		if cfg.GenMappingFileOnly {
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
				} else {
					messages = append(messages, fmt.Sprintf("(%d/%d) Importing %s as %s", idx+1, len(list), list[idx].TFResourceId, list[idx].TFAddr))
				}
				importList = append(importList, &list[idx])
			}

			msg.SetStatus(strings.Join(messages, "\n"))
			if err := c.ParallelImport(ctx, importList); err != nil {
				return fmt.Errorf("parallel importing: %v", err)
			}

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

		if err := c.PushState(ctx); err != nil {
			return fmt.Errorf("failed to push state: %v", err)
		}

		msg.SetStatus("Generating Terraform configurations...")
		if err := c.GenerateCfg(ctx, list); err != nil {
			return fmt.Errorf("generating Terraform configuration: %v", err)
		}

		msg.SetStatus("Cleaning up...")
		if err := c.CleanUpWorkspace(ctx); err != nil {
			return fmt.Errorf("cleaning up main workspace: %v", err)
		}

		return nil
	}

	var err error
	if cfg.PlainUI {
		err = f(NewStdoutMessager())
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
