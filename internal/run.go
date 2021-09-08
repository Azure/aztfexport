package internal

import (
	"context"
	"io"
	"log"

	"github.com/fatih/color"
)

func Run(ctx context.Context, rg string) error {
	// Discard the log output from the go-azure-helpers
	log.SetOutput(io.Discard)

	meta, err := NewMeta(ctx, rg)
	if err != nil {
		return err
	}

	if err := meta.InitProvider(ctx); err != nil {
		return err
	}

	if err := meta.ExportArmTemplate(ctx); err != nil {
		return err
	}

	ids := meta.ListAzureResourceIDs()

	// Repeat importing resources here to avoid the user incorrectly maps an azure resource to an incorrect terraform resource
	var importedList ImportList
	for len(ids) != 0 {
		l, err := meta.ResolveImportList(ids, ctx)
		if err != nil {
			return err
		}

		l, err = meta.Import(ctx, l)
		if err != nil {
			return err
		}

		for _, item := range l.Imported() {
			importedList = append(importedList, item)
		}

		importErroredList := l.ImportErrored()
		ids = make([]string, 0, len(importErroredList))
		for _, item := range importErroredList {
			ids = append(ids, item.ResourceID)
		}
	}

	configs, err := meta.StateToConfig(ctx, importedList)
	if err != nil {
		return err
	}

	configs, err = meta.ResolveDependency(ctx, configs)
	if err != nil {
		return err
	}

	if err := meta.GenerateConfig(configs); err != nil {
		return err
	}

	color.Cyan("\nPlease find the Terraform state and the config at: %s\n", meta.workspace)

	return nil
}
