package armtemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/aztfy/internal/client"
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

// ProviderId converts the ARM ResourceId to its ARM resource ID literal, based on the specified subscription id and resource
// group name. Then it will optionally transform the ARM resource ID to its corresponding TF resource ID, which might requires
// API interaction with ARM, where a non-nil client builder is required.
func (res ResourceId) ProviderId(sub, rg string, b *client.ClientBuilder) (string, error) {
	switch res.Type {
	case "microsoft.insights/webtests":
		return res.providerIdForInsightsWebtests(sub, rg, b)
	case "Microsoft.KeyVault/vaults/keys",
		"Microsoft.KeyVault/vaults/secrets",
		"Microsoft.KeyVault/vaults/certificates":
		return res.providerIdForKeyVaultNestedItems(sub, rg, b)
	default:
		return res.ID(sub, rg), nil
	}
}

func (res ResourceId) providerIdForInsightsWebtests(sub, rg string, b *client.ClientBuilder) (string, error) {
	// See issue: https://github.com/Azure/aztfy/issues/89
	res.Type = "Microsoft.insights/webTests"
	return res.ID(sub, rg), nil
}

func (res ResourceId) providerIdForKeyVaultNestedItems(sub, rg string, b *client.ClientBuilder) (string, error) {
	// See issue: https://github.com/Azure/aztfy/issues/86
	ctx := context.Background()
	switch res.Type {
	case "Microsoft.KeyVault/vaults/keys":
		client, err := b.NewKeyvaultKeysClient(sub)
		if err != nil {
			return "", err
		}
		segs := strings.Split(res.Name, "/")
		if len(segs) != 2 {
			return "", fmt.Errorf("malformed resource name %q for %q", res.Type, res.Name)
		}
		resp, err := client.Get(ctx, rg, segs[0], segs[1], nil)
		if err != nil {
			return "", fmt.Errorf("retrieving %s: %v", res.ID(sub, rg), err)
		}
		if resp.Key.Properties == nil || resp.Key.Properties.KeyURIWithVersion == nil {
			return "", fmt.Errorf("failed to get data plane URI from the response for %s", res.ID(sub, rg))
		}
		return *resp.Key.Properties.KeyURIWithVersion, nil
	case "Microsoft.KeyVault/vaults/secrets":
		client, err := b.NewKeyvaultSecretsClient(sub)
		if err != nil {
			return "", err
		}
		segs := strings.Split(res.Name, "/")
		if len(segs) != 2 {
			return "", fmt.Errorf("malformed resource name %q for %q", res.Type, res.Name)
		}
		resp, err := client.Get(ctx, rg, segs[0], segs[1], nil)
		if err != nil {
			return "", fmt.Errorf("retrieving %s: %v", res.ID(sub, rg), err)
		}
		if resp.Secret.Properties == nil || resp.Secret.Properties.SecretURIWithVersion == nil {
			return "", fmt.Errorf("failed to get data plane URI from the response for %s", res.ID(sub, rg))
		}
		return *resp.Secret.Properties.SecretURIWithVersion, nil
	case "Microsoft.KeyVault/vaults/certificates":
		// There is no such type called "Microsoft.KeyVault/vaults/certificates" in ARM, this is just a hypothetic type to indicate
		// the current resoruce corresponds to a certificate in data plane.
		// We will use secret client to get its secret resource from control plane, and then construct the uri from the data plane uri of the secret.
		client, err := b.NewKeyvaultSecretsClient(sub)
		if err != nil {
			return "", err
		}
		segs := strings.Split(res.Name, "/")
		if len(segs) != 2 {
			return "", fmt.Errorf("malformed resource name %q for %q", res.Type, res.Name)
		}
		resp, err := client.Get(ctx, rg, segs[0], segs[1], nil)
		if err != nil {
			return "", fmt.Errorf("retrieving %s: %v", res.ID(sub, rg), err)
		}
		if resp.Secret.Properties == nil || resp.Secret.Properties.SecretURIWithVersion == nil {
			return "", fmt.Errorf("failed to get data plane URI from the response for %s (secret actually)", res.ID(sub, rg))
		}
		id := *resp.Secret.Properties.SecretURIWithVersion
		segs = strings.Split(id, "/")
		segs[len(segs)-3] = "certificates"
		return strings.Join(segs, "/"), nil
	}
	panic("never reach here")
}
