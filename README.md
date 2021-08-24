## Azure Terrafy

A tool to bring your existing Azure resources into the management of Terraform.

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

[![asciicast](https://asciinema.org/a/432117.svg)](https://asciinema.org/a/432117)
