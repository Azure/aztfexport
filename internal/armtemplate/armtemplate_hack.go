package armtemplate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// TweakResources tweaks the resource set exported from ARM template, due to Terraform models the resources differently.
func (tpl *Template) TweakResources() error {
	// KeyVault certificate is a special resource that its data plane entity is composed of two control plane resources.
	// ARM template exports the control plane resource ids, while Terraform uses its data plane counterpart.
	if err := tpl.tweakForKeyVaultCertificate(); err != nil {
		return err
	}

	// Populate exclusively managed resources that are missing from ARM template.
	if err := tpl.populateManagedResources(); err != nil {
		return err
	}

	// For resources with no dependency, add the resource group to the depends on list.
	var newResources []Resource
	for _, res := range tpl.Resources {
		if len(res.DependsOn) == 0 {
			res.DependsOn = []ResourceId{
				{},
			}
		}
		newResources = append(newResources, res)
	}

	// Populate the resource group into the resouce list
	newResources = append(newResources, Resource{
		ResourceId: ResourceId{},
		DependsOn:  []ResourceId{},
	})

	tpl.Resources = newResources

	return nil
}

func (tpl *Template) tweakForKeyVaultCertificate() error {
	newResoruces := []Resource{}
	pending := map[string]Resource{}
	for _, res := range tpl.Resources {
		if res.Type != "Microsoft.KeyVault/vaults/keys" && res.Type != "Microsoft.KeyVault/vaults/secrets" {
			newResoruces = append(newResoruces, res)
			continue
		}
		if _, ok := pending[res.Name]; !ok {
			pending[res.Name] = res
			continue
		}
		delete(pending, res.Name)
		newResoruces = append(newResoruces, Resource{
			ResourceId: ResourceId{
				Type: "Microsoft.KeyVault/vaults/certificates",
				Name: res.Name,
			},
			Properties: nil,
			DependsOn:  res.DependsOn,
		})
	}
	for _, res := range pending {
		newResoruces = append(newResoruces, res)
	}
	tpl.Resources = newResoruces
	return nil
}

func (tpl *Template) populateManagedResources() error {
	newResoruces := []Resource{}
	knownManagedResourceTypes := map[string][]string{
		"Microsoft.Compute/virtualMachines": {
			"storageProfile.dataDisks.#.managedDisk.id",
		},
	}
	for _, res := range tpl.Resources {
		if paths, ok := knownManagedResourceTypes[res.Type]; ok {
			res, resources, err := populateManagedResourcesByPath(res, paths...)
			if err != nil {
				return fmt.Errorf(`populating managed resources for %q: %v`, res.Type, err)
			}
			newResoruces = append(newResoruces, *res)
			newResoruces = append(newResoruces, resources...)
		} else {
			newResoruces = append(newResoruces, res)
		}
	}
	tpl.Resources = newResoruces
	return nil
}

// populateManagedResourcesByPath populate the managed resources in the specified paths.
// It will also update the specified resource's dependency accordingly.
func populateManagedResourcesByPath(res Resource, paths ...string) (*Resource, []Resource, error) {
	b, err := json.Marshal(res.Properties)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling %v: %v", res.Properties, err)
	}
	var resources []Resource
	for _, path := range paths {
		result := gjson.GetBytes(b, path)
		if !result.Exists() {
			continue
		}

		for _, exprResult := range result.Array() {
			// ARM template export ids in two forms:
			// - Call expression: [resourceids(type, args)]. This is for resources within current export scope.
			// - Id literal: This is for resources beyond current export scope .
			if !strings.HasPrefix(exprResult.String(), "[") {
				continue
			}
			id, err := ParseResourceIdFromCallExpr(exprResult.String())
			if err != nil {
				return nil, nil, err
			}

			// Ideally, we should recursively export ARM template for this resource, fill in its properties
			// and populate any managed resources within it, unless it has already exported.
			// But here, as we explicitly pick up the managed resource to be populated, which means it is rarely possible that
			// these resource are exported by the ARM template.
			// TODO: needs to recursively populate these resources?
			mres := Resource{
				ResourceId: ResourceId{
					Type: id.Type,
					Name: id.Name,
				},
				DependsOn: []ResourceId{},
			}
			res.DependsOn = append(res.DependsOn, mres.ResourceId)
			resources = append(resources, mres)
		}
	}
	return &res, resources, nil
}
