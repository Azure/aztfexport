package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/tfaddr"
	"github.com/Azure/aztfy/internal/ui/common"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	"github.com/magodo/spinner"
)

func ResourceImport(cfg config.ResConfig) error {
	c, err := meta.NewResMeta(cfg)
	if err != nil {
		return err
	}

	s := bspinner.NewModel()
	s.Spinner = common.Spinner

	return spinner.Run(s, func(msg spinner.Messager) error {
		msg.SetStatus("Initializing...")
		if err := c.Init(); err != nil {
			return err
		}

		item := meta.ImportItem{
			TFAddr: tfaddr.TFAddr{
				Name: c.ResourceName,
			},
		}

		if c.ResourceType == "" {
			msg.SetStatus("Querying Terraform resource type and id...")
			rt, tfid, err := c.QueryResourceTypeAndId()
			if err != nil {
				return err
			}
			item.ResourceID = tfid
			item.TFAddr.Type = rt
		} else {
			msg.SetStatus("Querying Terraform resource id...")
			tfid, err := c.QueryResourceId(c.ResourceType)
			if err != nil {
				return err
			}
			item.ResourceID = tfid
			item.TFAddr.Type = c.ResourceType
		}

		msg.SetDetail(fmt.Sprintf(`Resource Type: %s
Resource Id  : %s`, item.TFAddr.Type, item.ResourceID))

		msg.SetStatus("Importing...")
		c.Import(&item)
		if err := item.ImportError; err != nil {
			return fmt.Errorf("failed to import %s as %s: %v", item.ResourceID, item.TFAddr, err)
		}

		msg.SetStatus("Generating Terraform configurations...")
		if err := c.GenerateCfg(meta.ImportList{item}); err != nil {
			return fmt.Errorf("generating Terraform configuration: %v", err)
		}

		return nil
	})
}

func BatchImport(cfg config.GroupConfig, continueOnError bool) error {
	c, err := meta.NewGroupMeta(cfg)
	if err != nil {
		return err
	}

	s := bspinner.NewModel()
	s.Spinner = common.Spinner

	var warnings []string
	err = spinner.Run(s, func(msg spinner.Messager) error {
		msg.SetStatus("Initializing...")
		if err := c.Init(); err != nil {
			return err
		}

		msg.SetStatus("Listing resources...")
		list, err := c.ListResource()
		if err != nil {
			return err
		}

		msg.SetStatus("Importing resources...")
		for i := range list {
			if list[i].Skip() {
				warnings = append(warnings, fmt.Sprintf("No mapping information for resource: %s, skip it", list[i].ResourceID))
				msg.SetDetail(strings.Join(warnings, "\n"))
				continue
			}
			msg.SetStatus(fmt.Sprintf("(%d/%d) Importing %s as %s", i+1, len(list), list[i].ResourceID, list[i].TFAddr))
			c.Import(&list[i])
			if err := list[i].ImportError; err != nil {
				msg := fmt.Sprintf("Failed to import %s as %s: %v", list[i].ResourceID, list[i].TFAddr, err)
				if !continueOnError {
					return fmt.Errorf(msg)
				}
				warnings = append(warnings, msg)
			}
		}

		msg.SetStatus("Generating Terraform configurations...")
		if err := c.GenerateCfg(list); err != nil {
			return fmt.Errorf("generating Terraform configuration: %v", err)
		}
		return nil
	})

	// Print out the warnings, if any
	if len(warnings) != 0 {
		fmt.Fprintln(os.Stderr, "Warnings:\n"+strings.Join(warnings, "\n"))
	}

	return err
}
