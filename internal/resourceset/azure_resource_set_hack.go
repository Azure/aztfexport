package resourceset

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/magodo/armid"

	"github.com/tidwall/gjson"
)

// PopulateResourceTypes is a map to record resources that might to be populate other resources.
// This is used in single resource mode to decide whether to call API to get its body.
var PopulateResourceTypes = map[string]bool{
	"MICROSOFT.COMPUTE/VIRTUALMACHINES": true,
}

// PopulateResource populate single resource for certain Azure resouce type that is known might maps to more than one TF resources, which are missing from azlist.
// In most cases, this step is used to populate the Azure managed resource.
func (rset *AzureResourceSet) PopulateResource() error {
	// Populate managed data disk (and the association) for VMs that are missing from Azure exported resource set.
	if err := rset.populateForVirtualMachine(); err != nil {
		return err
	}
	return nil
}

// ReduceResource reduce the resource set for certain multiple Azure resources that are known to be mapped to only one TF resource.
func (rset *AzureResourceSet) ReduceResource() error {
	// KeyVault certificate is a special resource that its data plane entity is composed of two control plane resources.
	// Azure exports the control plane resource ids, while Terraform uses its data plane counterpart.
	if err := rset.reduceForKeyVaultCertificate(); err != nil {
		return err
	}
	return nil
}

func (rset *AzureResourceSet) reduceForKeyVaultCertificate() error {
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

func (rset *AzureResourceSet) populateForVirtualMachine() error {
	for _, res := range rset.Resources[:] {
		if strings.ToUpper(res.Id.RouteScopeString()) != "/MICROSOFT.COMPUTE/VIRTUALMACHINES" {
			continue
		}
		disks, err := populateManagedResourcesByPath(res, "properties.storageProfile.dataDisks.#.managedDisk.id")
		if err != nil {
			return fmt.Errorf(`populating managed disks for %q: %v`, res.Id, err)
		}
		rset.Resources = append(rset.Resources, disks...)
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
