package resourceset

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/magodo/armid"

	"github.com/tidwall/gjson"
)

// TweakResources tweaks the resource set exported from Azure, due to Terraform models the resources differently.
func (rset *AzureResourceSet) TweakResources() error {
	if err := rset.ARMTemplateTweak(); err != nil {
		return fmt.Errorf("tweaking resource set as ARM template: %v", err)
	}

	// KeyVault certificate is a special resource that its data plane entity is composed of two control plane resources.
	// Azure exports the control plane resource ids, while Terraform uses its data plane counterpart.
	if err := rset.tweakForKeyVaultCertificate(); err != nil {
		return err
	}

	// Populate exclusively managed resources that are missing from Azure exported resource set.
	//if err := rset.populateManagedResources(); err != nil {
	//	return err
	//}

	return nil
}

// Mimic how ARM template tweaks the resource list result.
func (rset *AzureResourceSet) ARMTemplateTweak() error {
	// Remove managedBy resources no matter the managing resource is in the resource set.
	// Since these resources should be totally managed by other resources in its lifecycle, rather than by users.
	rl := []AzureResource{}
	for _, res := range rset.Resources {
		if v, ok := res.Properties["managedBy"]; ok && v.(string) != "" {
			continue
		}
		rl = append(rl, res)
	}

	// Recursively add direct child resources if the child resource has all its properties defined as a property within the parent resource.
	nrl := []AzureResource{}

	// addChildFromProp discover direct child for the given resource, adding the child resource to the nrl defined above.
	var addChildFromProp func(armid.ResourceId, interface{})

	addChildFromProp = func(pid armid.ResourceId, p interface{}) {
		if array, ok := p.([]interface{}); ok {
			for _, e := range array {
				addChildFromProp(pid, e)
			}
			return
		}

		prop, ok := p.(map[string]interface{})
		if !ok {
			return
		}

		id, ok := prop["id"]
		if !ok {
			// If there is no "id" defined in this layer, then continue
			for _, v := range prop {
				addChildFromProp(pid, v)
			}
			return
		}

		// If "id" is found, then ensure it is the direct child resource of the current resource.
		rid, err := armid.ParseResourceId(id.(string))
		if err != nil {
			// If this is not a valid azure id (e.g. maybe this is some UUID), then continue
			for _, v := range prop {
				addChildFromProp(pid, v)
			}
			return
		}

		parent := rid.Parent()
		if parent == nil || !parent.Equal(pid) {
			// This is not a direct child resource of the current resource, ignore it
			return
		}

		// This is a direct child resource. Then we should further ensure this child resource is defined inline, other than referenced by the parent resource.
		// We simply check whether this layer is of the properties defined for a typical azure resource schema.
		if _, ok := prop["name"]; !ok {
			return
		}
		if _, ok := prop["type"]; !ok {
			return
		}

		nrl = append(nrl, AzureResource{
			Id:         rid,
			Properties: prop,
		})

		// Recursively add the grand child resource from this child resource
		addChildFromProp(rid, prop)
	}

	for _, res := range rl {
		nrl = append(nrl, res)

		prop, ok := res.Properties["properties"]
		if !ok {
			// Though not likely to happen
			continue
		}

		addChildFromProp(res.Id, prop)
	}

	rset.Resources = nrl
	return nil
}

func (rset *AzureResourceSet) tweakForKeyVaultCertificate() error {
	newResoruces := []AzureResource{}
	pending := map[string]AzureResource{}
	for _, res := range rset.Resources {
		if !strings.EqualFold(res.Id.RouteScopeString(), "/Microsoft.KeyVault/vaults/keys") && !strings.EqualFold(res.Id.RouteScopeString(), "/Microsoft.KeyVault/vaults/secrets") {
			newResoruces = append(newResoruces, res)
			continue
		}
		names := res.Id.Names()
		certName := names[len(names)-1]
		if _, ok := pending[certName]; !ok {
			pending[certName] = res
			continue
		}
		delete(pending, certName)
		certId := res.Id.Clone().(*armid.ScopedResourceId)
		certId.AttrTypes[len(certId.AttrTypes)-1] = "certificates"
		newResoruces = append(newResoruces, AzureResource{
			Id: certId,
		})
	}
	for _, res := range pending {
		newResoruces = append(newResoruces, res)
	}
	rset.Resources = newResoruces
	return nil
}

func (rset *AzureResourceSet) populateManagedResources() error {
	newResoruces := []AzureResource{}
	knownManagedResourceTypes := map[string][]string{
		"/MICROSOFT.COMPUTE/VIRTUALMACHINES": {
			"storageProfile.dataDisks.#.managedDisk.id",
		},
	}
	for _, res := range rset.Resources {
		if paths, ok := knownManagedResourceTypes[strings.ToUpper(res.Id.RouteScopeString())]; ok {
			resources, err := populateManagedResourcesByPath(res, paths...)
			if err != nil {
				return fmt.Errorf(`populating managed resources for %q: %v`, res.Id, err)
			}
			newResoruces = append(newResoruces, resources...)
		} else {
			newResoruces = append(newResoruces, res)
		}
	}
	rset.Resources = newResoruces
	return nil
}

// populateManagedResourcesByPath populate the managed resources in the specified paths.
func populateManagedResourcesByPath(res AzureResource, paths ...string) ([]AzureResource, error) {
	b, err := json.Marshal(res.Properties)
	if err != nil {
		return nil, fmt.Errorf("marshaling %v: %v", res.Properties, err)
	}
	var resources []AzureResource
	for _, path := range paths {
		result := gjson.GetBytes(b, path)
		if !result.Exists() {
			continue
		}

		for _, exprResult := range result.Array() {
			mid := exprResult.String()
			id, err := armid.ParseResourceId(mid)
			if err != nil {
				return nil, fmt.Errorf("parsing managed resource id %s: %v", mid, err)
			}
			resources = append(resources, AzureResource{
				Id: id,
			})
		}
	}
	return resources, nil
}
