package internal

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/resourceset"
	"github.com/Azure/aztfy/internal/tfaddr"
	"github.com/Azure/aztfy/internal/ui/common"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/magodo/spinner"
)

func ResourceImport(ctx context.Context, cfg config.ResConfig, continueOnError bool) error {
	c, err := meta.NewResMeta(cfg)
	if err != nil {
		return err
	}

	var errors []string

	f := func(msg Messager) error {
		msg.SetStatus("Initializing...")
		if err := c.Init(); err != nil {
			return err
		}

		resourceSet := resourceset.AzureResourceSet{
			Resources: []resourceset.AzureResource{
				{
					Id: c.AzureId,
				},
			},
		}

		// Populating resource requires API body. We only call GET on the known resources because:
		// 1. The ARM schema API version might be wrong
		// 2. Resoruce mode supports pesudo resources defined by aztft (e.g. key vault certificate), which has no Azure counterpart
		if resourceset.PopulateResourceTypes[strings.ToUpper(c.AzureId.TypeString())] {
			body, err := c.GetAzureResource(ctx)
			if err != nil {
				return err
			}
			resourceSet.Resources[0].Properties = body
			if err := resourceSet.PopulateResource(); err != nil {
				return err
			}
		}

		msg.SetStatus("Querying Terraform resource type and id...")
		rl := resourceSet.ToTFResources()

		var l meta.ImportList
		for _, res := range rl {
			item := meta.ImportItem{
				AzureResourceID: res.AzureId,
				TFResourceId:    res.TFId, // this might be empty if have multiple matches in aztft
				TFAddr: tfaddr.TFAddr{
					Type: res.TFType, //this might be empty if have multiple matches in aztft
					Name: c.ResourceName,
				},
			}

			// Some special Azure resource is missing the essential property that is used by aztft to detect their TF resource type.
			// In this case, users can use the `--type` option to manually specify the TF resource type.
			if c.ResourceType != "" {
				if c.AzureId.Equal(res.AzureId) {
					tfid, err := c.QueryResourceId(c.ResourceType)
					if err != nil {
						return err
					}
					item.TFResourceId = tfid
					item.TFAddr.Type = c.ResourceType
				}
			}

			l = append(l, item)
		}

		msg.SetStatus("Exporting Resource Mapping file...")
		if err := c.ExportResourceMapping(l); err != nil {
			return fmt.Errorf("exporting Resource Mapping file: %v", err)
		}

		// Return early if only generating mapping file
		if cfg.GenerateMappingFile {
			return nil
		}

		msgs := []string{}
		for _, item := range l {
			msgs = append(msgs, fmt.Sprintf(`Resource Address: %s
Resource Id  : %s`, item.TFAddr, item.TFResourceId))
		}
		msg.SetDetail(strings.Join(msgs, "\n\n"))

		for i := range l {
			item := &l[i]
			msg.SetStatus(fmt.Sprintf("(%d/%d) Importing %s as %s", i+1, len(l), item.TFResourceId, item.TFAddr))
			c.Import(item)
			if err := item.ImportError; err != nil {
				msg := fmt.Sprintf("Failed to import %s as %s: %v", item.TFResourceId, item.TFAddr, err)
				if !continueOnError {
					return fmt.Errorf(msg)
				}
				errors = append(errors, msg)
			}
		}

		msg.SetStatus("Generating Terraform configurations...")
		if err := c.GenerateCfg(l); err != nil {
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

func BatchImport(cfg config.GroupConfig, continueOnError bool) error {
	c, err := meta.NewGroupMeta(cfg)
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
				if !continueOnError {
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
