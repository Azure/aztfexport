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
	oldResources := make([]Resource, len(tpl.Resources))
	copy(oldResources, tpl.Resources)
	for _, res := range oldResources {
		switch res.Type {
		case "Microsoft.Compute/virtualMachines":
			resources, err := populateManagedResources(res.Properties, "storageProfile.dataDisks.#.managedDisk.id")
			if err != nil {
				return fmt.Errorf(`populating managed resources for "Microsoft.Compute/virtualMachines": %v`, err)
			}
			tpl.Resources = append(tpl.Resources, resources...)
		}
	}
	return nil
}

func populateManagedResources(props interface{}, paths ...string) ([]Resource, error) {
	b, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("marshaling %v: %v", props, err)
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
				return nil, err
			}

			// Ideally, we should recursively export ARM template for this resource, fill in its properties
			// and populate any managed resources within it, unless it has already exported.
			// But here, as we explicitly pick up the managed resource to be populated, which means it is rarely possible that
			// these resource are exported by the ARM template.
			// TODO: needs to recursively populate these resources?
			res := Resource{
				ResourceId: ResourceId{
					Type: id.Type,
					Name: id.Name,
				},
				DependsOn: []ResourceId{},
			}
			resources = append(resources, res)
		}
	}
	return resources, nil
}
