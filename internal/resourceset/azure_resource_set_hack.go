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
	// KeyVault certificate is a special resource that its data plane entity is composed of two control plane resources.
	// Azure exports the control plane resource ids, while Terraform uses its data plane counterpart.
	if err := rset.tweakForKeyVaultCertificate(); err != nil {
		return err
	}

	// Populate exclusively managed resources that are missing from Azure exported resource set.
	if err := rset.populateManagedResources(); err != nil {
		return err
	}

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
	knownManagedResourceTypes := map[string][]string{
		"/MICROSOFT.COMPUTE/VIRTUALMACHINES": {
			"properties.storageProfile.dataDisks.#.managedDisk.id",
		},
	}
	for _, res := range rset.Resources[:] {
		if paths, ok := knownManagedResourceTypes[strings.ToUpper(res.Id.RouteScopeString())]; ok {
			resources, err := populateManagedResourcesByPath(res, paths...)
			if err != nil {
				return fmt.Errorf(`populating managed resources for %q: %v`, res.Id, err)
			}
			rset.Resources = append(rset.Resources, resources...)
		}
	}
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
