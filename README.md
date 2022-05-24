## Azure Terrafy

A tool to bring your existing Azure resources under the management of Terraform.

## Goal

Azure Terrafy imports the resources that are supported by the [Terraform AzureRM provider](https://github.com/hashicorp/terraform-provider-azurerm) within a resource group, into the Terraform state, and generates the corresponding Terraform configuration. Both the Terraform state and configuration are expected to be consistent with the resources' remote state, i.e., `terraform plan` shows no diff. The user then is able to use Terraform to manage these resources.

## Install

### From Release

Precompiled binaries are available at [Releases](https://github.com/Azure/aztfy/releases).

### From Go toolchain

```bash
go install github.com/Azure/aztfy@latest
```

## Precondition

There is no special precondtion needed for running `aztfy`, except that you have access to Azure.

Although `aztfy` depends on `terraform`, it is not required to have `terraform` pre-installed and configured in the [`PATH`](https://en.wikipedia.org/wiki/PATH_(variable)) before running `aztfy`. `aztfy` will ensure a `terraform` in the following order: 

- If there is already a `terraform` discovered in the `PATH` whose version `>= v0.12`, then use it
- Otherwise, if there is already a `terraform` installed at the `aztfy` cache directory, then use it
- Otherwise, install the latest `terraform` from Hashicorp's release to the `aztfy` cache directory

(The `aztfy` cache directory is at: "[\<UserCacheDir\>](https://pkg.go.dev/os#UserCacheDir)/aztfy")

## Usage

Follow the [authentication guide](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#authenticating-to-azure) from the Terraform AzureRM provider to authenticate to Azure.

Then you can go ahead and run `aztfy [option] <resource group name>`. The tool can run in two modes: interactive mode and batch mode, depending on whether `-b` is specified.

### Interactive Mode

In interactive mode, `aztfy` list all the resources resides in the specified resource group. For each resource, user is expected to input the Terraform resource address in form of `<resource type>.<resource name>` (e.g. `azurerm_linux_virtual_machine.test`). Users can press `r` to see the possible resource type(s) for the selected import item (though this is not guaranteed to be 100% accurate). In case there is exactly one resource type match for the import item, that resource type will be automatically filled in the text input for the users, with a 💡 line prefix as an indication.

In some cases, there are Azure resources that have no corresponding Terraform resource (e.g. due to lacks of Terraform support), or some resource might be created as a side effect of provisioning another resource (e.g. the OS Disk resource is created automatically when provisioning a VM). In these cases, you can skip these resources without typing anything.

> 💡 Option `-m` can be used to specify a resource mapping file, either constructed manually or from other runs of `aztfy` (generated in the output directory with name: _.aztfyResourceMapping.json_).

After going through all the resources to be imported, users press `w` to instruct `aztfy` to proceed importing resources into Terraform state and generating the Terraform configuration.

> 💡 `aztfy` will run `terraform import` under the hood to import each resource. Then it will run [`tfadd`](https://github.com/magodo/tfadd) to generate the Terraform template for each imported resource. Whereas there are kinds of [quirks](https://github.com/apparentlymart/terrafy/blob/main/docs/quirks.md) causing the output of `tfadd` to be an invalid Terraform template in most cases. `aztfy` will leverage extra knowledge from the provider (which is generated from the provider codebase) to further manipulate the template, to make it pass the Terraform validations against the provider.
> 
> As the last step, `aztfy` will leverage the ARM template to inject dependencies between each resource. This makes the generated Terraform template to be useful.

### Batch Mode

In batch mode, instead of interactively specifying the mapping from Azurem resource id to the Terraform resource address, users are expected to provide that mapping via the resource mapping file (via `-m`), with the following format:

```json
{
    "<azure resource id1>": "<terraform resource type1>.<terraform resource name>",
    "<azure resource id2>": "<terraform resource type2>.<terraform resource name>",
    ...
}
```

Example:

```json
{
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/virtualNetworks/example-network": "azurerm_virtual_network.res-0",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Compute/virtualMachines/example-machine": "azurerm_linux_virtual_machine.res-1",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/networkInterfaces/example-nic": "azurerm_network_interface.res-2",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/networkInterfaces/example-nic1": "azurerm_network_interface.res-3",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/virtualNetworks/example-network/subnets/internal": "azurerm_subnet.res-4"
}
```

Then the tool will import each specified resource in the mapping file (if exists) and skip the others.

Especially if the no resource mapping file is specified, `aztfy` will only import the "recognized" resources for you, based on its limited knowledge on the ARM and Terraform resource mappings.

In the batch import mode, users can further specify the `-k` option to make the tool continue even on hitting import error(s) on any resource.

## Demo

[![asciicast](https://asciinema.org/a/475516.svg)](https://asciinema.org/a/475516)

## Limitation

Some Azure resources are modeled differently in AzureRM provider, which means there might be N:M mapping between the Azure resources and the Terraform resources.

For example, the `azurerm_lb_backend_address_pool_address` is actually a property of `azurerm_lb_backend_address_pool`, whilst in the AzureRM provider, it has its own resource and a synthetic resource ID as `/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/group1/providers/Microsoft.Network/loadBalancers/loadBalancer1/backendAddressPools/backendAddressPool1/addresses/address1`.

Another popular case is that in the AzureRM provider, there are a bunch of "association" resources, e.g. the `azurerm_network_interface_security_group_association`. These "association" resources represent the association relationship between two Terraform resources (in this case they are `azurerm_network_interface` and `azurerm_network_security_group`). They also have some synthetic resource ID, e.g. `/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/mygroup1/providers/microsoft.network/networkInterfaces/example|/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/group1/providers/Microsoft.Network/networkSecurityGroups/group1`.

Currently, this tool only works on the assumption that there is 1:1 mapping between Azure resources and the Terraform resources.

## Additional Resources

- [The aztfy Github Page](https://azure.github.io/aztfy): Everything about aztfy, including comparisons with other existing import solutions.
- [Kyle Ruddy's Blog about aztfy](https://www.kmruddy.com/2022/terrafy-existing-azure-resources/): A live use of `aztfy`, explaining the pros and cons.
