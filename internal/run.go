package internal

import (
	"fmt"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/internal/tfaddr"
)

func ResourceImport(cfg config.ResConfig) error {
	c, err := meta.NewResMeta(cfg)
	if err != nil {
		return err
	}
	fmt.Println("Initializing...")
	if err := c.Init(); err != nil {
		return err
	}

	fmt.Println("Querying Terraform resource type and id...")
	rt, tfid, err := c.QueryResourceTypeAndId()
	if err != nil {
		return err
	}

	item := meta.ImportItem{
		ResourceID: tfid,
		TFAddr: tfaddr.TFAddr{
			Type: rt,
			Name: c.ResourceName,
		},
	}

	fmt.Printf("\nResource type: %s\nResource Id: %s\n\n", item.TFAddr.Type, item.ResourceID)
	fmt.Println("Importing...")
	c.Import(&item)
	if err := item.ImportError; err != nil {
		return fmt.Errorf("failed to import %s as %s: %v", item.ResourceID, item.TFAddr, err)
	}

	fmt.Println("Generating Terraform configurations...")
	if err := c.GenerateCfg(meta.ImportList{item}); err != nil {
		return fmt.Errorf("generating Terraform configuration: %v", err)
	}

	return nil
}

func BatchImport(cfg config.RgConfig, continueOnError bool) error {
	c, err := meta.NewRgMeta(cfg)
	if err != nil {
		return err
	}

	fmt.Println("Initializing...")
	if err := c.Init(); err != nil {
		return err
	}

	fmt.Println("List resources...")
	list, err := c.ListResource()
	if err != nil {
		return err
	}

	fmt.Println("Import resources...")
	for i := range list {
		if list[i].Skip() {
			fmt.Printf("[WARN] No mapping information for resource: %s, skip it\n", list[i].ResourceID)
			continue
		}
		fmt.Printf("Importing %s as %s\n", list[i].ResourceID, list[i].TFAddr)
		c.Import(&list[i])
		if err := list[i].ImportError; err != nil {
			msg := fmt.Sprintf("Failed to import %s as %s: %v", list[i].ResourceID, list[i].TFAddr, err)
			if !continueOnError {
				return fmt.Errorf(msg)
			}
			fmt.Println("[ERROR] " + msg)
		}
	}

	fmt.Println("Generating Terraform configurations...")
	if err := c.GenerateCfg(list); err != nil {
		return fmt.Errorf("generating Terraform configuration: %v", err)
	}

	return nil
}
