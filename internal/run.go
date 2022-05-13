package internal

import (
	"fmt"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/meta"
)

func BatchImport(cfg config.Config, continueOnError bool) error {
	c, err := meta.NewMeta(cfg)
	if err != nil {
		return err
	}

	fmt.Println("Initializing...")
	if err := c.Init(); err != nil {
		return err
	}

	fmt.Println("List resources...")
	list := c.ListResource()

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
