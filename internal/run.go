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

	f := func(msg Messager) error {
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
			item.TFResourceId = tfid
			item.TFAddr.Type = rt
		} else {
			msg.SetStatus("Querying Terraform resource id...")
			tfid, err := c.QueryResourceId(c.ResourceType)
			if err != nil {
				return err
			}
			item.TFResourceId = tfid
			item.TFAddr.Type = c.ResourceType
		}

		msg.SetDetail(fmt.Sprintf(`Resource Type: %s
Resource Id  : %s`, item.TFAddr.Type, item.TFResourceId))

		msg.SetStatus("Importing...")
		c.Import(&item)
		if err := item.ImportError; err != nil {
			return fmt.Errorf("failed to import %s as %s: %v", item.TFResourceId, item.TFAddr, err)
		}

		msg.SetStatus("Generating Terraform configurations...")
		if err := c.GenerateCfg(meta.ImportList{item}); err != nil {
			return fmt.Errorf("generating Terraform configuration: %v", err)
		}

		return nil
	}

	if cfg.PlainUI {
		return f(&StdoutMessager{})
	}

	s := bspinner.NewModel()
	s.Spinner = common.Spinner
	sf := func(msg spinner.Messager) error {
		return f(&msg)
	}
	return spinner.Run(s, sf)
}

func BatchImport(cfg config.GroupConfig, continueOnError bool) error {
	c, err := meta.NewGroupMeta(cfg)
	if err != nil {
		return err
	}

	var warnings []string

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

		msg.SetStatus("Importing resources...")
		for i := range list {
			if list[i].Skip() {
				warnings = append(warnings, fmt.Sprintf("No mapping information for resource: %s, skip it", list[i].TFResourceId))
				msg.SetDetail(strings.Join(warnings, "\n"))
				continue
			}
			msg.SetStatus(fmt.Sprintf("(%d/%d) Importing %s as %s", i+1, len(list), list[i].TFResourceId, list[i].TFAddr))
			c.Import(&list[i])
			if err := list[i].ImportError; err != nil {
				msg := fmt.Sprintf("Failed to import %s as %s: %v", list[i].TFResourceId, list[i].TFAddr, err)
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

	// Print out the warnings, if any
	if len(warnings) != 0 {
		fmt.Fprintln(os.Stderr, "Warnings:\n"+strings.Join(warnings, "\n"))
	}

	return err
}
