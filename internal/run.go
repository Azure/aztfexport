package internal

import (
	"context"
	"io"
	"log"
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

	importList, err := meta.ResolveImportList(ctx)
	if err != nil {
		return err
	}

	if err := meta.Import(ctx, importList); err != nil {
		return err
	}

	configs, err := meta.StateToConfig(ctx, importList)
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

	return nil
}
