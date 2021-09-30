## Azure Terrafy

A tool to bring your existing Azure resources under the management of Terraform.

## Goal

Import all the AzureRM provider supported resources inside the resource group that the user specifies into to Terraform state, and generate a valid Terraform configuration. After running this tool, the Terraform state and configuration should be consistent with the resources' remote state, i.e. `terraform plan` shows no diff. The user then is able to use Terraform to manage these resources.

## Install

```bash
go install github.com/magodo/aztfy@latest
```

## Usage

Follow the [authentication guide](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#authenticating-to-azure) from the Terraform AzureRM provider to authenticate to Azure. The simplist way is to [install and login via the Azure CLI](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/guides/azure_cli).

Then you can go ahead and run:

```shell
$ aztfy <resource_group_name>
```

The tool will then list all the resources resides in the specified resource group. For each of them, it will ask the user to input the Terraform resource type and name for each Azure resource in the form of `<resource type>.<resource name>` (e.g. `azurerm_linux_virtual_machine.example`).

Especially, in some cases there are resources that have no corresponding terraform resource (e.g. due to lacks of Terraform support), or some resource might be created as a side effect of provisioning another resource (e.g. the Disk resource is created automatically when provisioning a VM). In these cases, you can directly press enter without typing anything to skip that resource.

After getting the input from user, `aztfy` will run `terraform import` to import each resource. Then it will run `terraform add -from-state` to generate the Terraform template for each imported resource. Where as there are kinds of [limitations](https://github.com/apparentlymart/terrafy/blob/main/docs/quirks.md) causing the output of `terraform add` to be an invalid Terraform template in most cases. `aztfy` will leverage extra knowledge from the provider (which is generated from the provider codebase) to further manipulate the template, to make it pass the terraform validations against the provider.

As a last step, `aztfy` will leverage the ARM template to inject dependencies between each resource. This makes the generated Terraform template to be useful.

## Demo

[![asciicast](https://asciinema.org/a/iPTGS6E2CSxpYPtbPQhWmxLdu.svg)](https://asciinema.org/a/iPTGS6E2CSxpYPtbPQhWmxLdu)

## Limitation

Some Azure resources are modeled differently in AzureRM provider, which means there might be N:M mapping between the Azure resources and the Terraform resources.

For example, the `azurerm_lb_backend_address_pool_address` is actually a property of `azurerm_lb_backend_address_pool`, whilst in the AzureRM provider, it has its own resource and a synthetic resource ID as `/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/group1/providers/Microsoft.Network/loadBalancers/loadBalancer1/backendAddressPools/backendAddressPool1/addresses/address1`.

Another popular case is that in the AzureRM provider, there are a bunch of "association" resources, e.g. the `azurerm_network_interface_security_group_association`. These "association" resources represent the association relationship between two Terraform resources (in this case they are `azurerm_network_interface` and `azurerm_network_security_group`). They also have some synthetic resource ID, e.g. `/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/mygroup1/providers/microsoft.network/networkInterfaces/example|/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/group1/providers/Microsoft.Network/networkSecurityGroups/group1`.

Currently, this tool only works on the assumption that there is 1:1 mapping between Azure resources and the Terraform resources.
