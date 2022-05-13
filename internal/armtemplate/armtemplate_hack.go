package armtemplate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// PopulateManagedResources populate extra resources that are missing from ARM template due to they are exclusively managed by another resource.
// E.g. managed disk resource id is not returned via ARM template as it is exclusively managed by VM.
// Terraform models the resources differently than ARM template and needs to import those managed resources separately.
func (tpl *Template) PopulateManagedResources() error {
	knownManagedResourceTypes := map[string][]string{
		"Microsoft.Compute/virtualMachines": {
			"storageProfile.dataDisks.#.managedDisk.id",
		},
	}

	newResoruces := []Resource{}
	for _, res := range tpl.Resources {
		if paths, ok := knownManagedResourceTypes[res.Type]; ok {
			res, resources, err := populateManagedResources(res, paths...)
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

// populateManagedResources populate the managed resources in the specified paths.
// It will also update the specified resource's dependency accordingly.
func populateManagedResources(res Resource, paths ...string) (*Resource, []Resource, error) {
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
			id, err := NewResourceIdFromCallExpr(exprResult.String())
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
