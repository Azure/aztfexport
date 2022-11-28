## Azure Terrafy

A tool to bring your existing Azure resources under the management of Terraform.

## Goal

Azure Terrafy imports the resources that are supported by the [Terraform AzureRM provider](https://github.com/hashicorp/terraform-provider-azurerm) into the Terraform state, and generates the corresponding Terraform configuration. Both the Terraform state and configuration are expected to be consistent with the resources' remote state, i.e., `terraform plan` shows no diff. The user then is able to use Terraform to manage these resources.

## Non Goal

The Terraform configurations generated by `aztfy` is not meant to be comprehensive. This means it doesn't guarantee the infrastruction can be reproduced via the generated configurations. For details, please refer to the [limitation](#limitation).

## Install

### From Release

Precompiled binaries and Window MSI are available at [Releases](https://github.com/Azure/aztfy/releases).

### From Go toolchain

```bash
go install github.com/Azure/aztfy@latest
```

### From Package Manager

#### Windows

```bash
winget install aztfy
```

#### Homebrew (Linux/macOS)

```bash
brew install aztfy
```

#### dnf (Linux)

Supported versions:

- RHEL 8 (amd64, arm64)
- RHEL 9 (amd64, arm64)

1. Import the Microsoft repository key:

    ```
    rpm --import https://packages.microsoft.com/keys/microsoft.asc
    ```

2. Add `packages-microsoft-com-prod` repository:

    ```
    ver=8 # or 9
    dnf install -y https://packages.microsoft.com/config/rhel/${ver}/packages-microsoft-prod.rpm
    ```

3. Install:

    ```
    dnf install aztfy
    ```

#### apt (Linux)

Supported versions:

- Ubuntu 20.04 (amd64, arm64)
- Ubuntu 22.04 (amd64, arm64)

1. Import the Microsoft repository key:

    ```Bash
    sudo sh -c 'curl -sSL https://packages.microsoft.com/keys/microsoft.asc > /etc/apt/trusted.gpg.d/microsoft.asc'
    ```

2. Add appropriate version of `packages-microsoft-com-prod` repository (either 20.04 or 22.04, script below retrieves Ubuntu version from lsb-release file and sets variable accordingly):

    ```Bash
    # ver value 20.04 or 22.04 gets retrieved from lsb-release file
    ver=$(grep "DISTRIB_RELEASE=" /etc/lsb-release | grep -E [0-9]{2}.[0-9]{2} -o)
    apt-add-repository https://packages.microsoft.com/ubuntu/${ver}/prod
    ```

3. Install:

    ```
    apt-get install aztfy
    ```

#### AUR (Linux)

```bash
yay -S aztfy
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

Then you can go ahead and run `aztfy resource <resource id>`, `aztfy resource-group <resource group name>` or `aztfy query <arg where predicate>` to import a single resource, a resource group including child resources, or a customized set of resources by an Azure Resource Graph query.

### Terrafy a Single Resource

`aztfy resource [option] <resource id>` terrafies a single resource by its Azure control plane ID.

E.g.

```shell
aztfy resource /subscriptions/0000/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1
```

The command will automatically identify the Terraform resource type (e.g. correctly identifies above resource as `azurerm_linux_virtual_machine`), and import it into state file and generate the Terraform configuration.

> ❗For data plane only or property-like resources, the Azure resource ID is using a pesudo format, as is defined [here](https://github.com/magodo/aztft#pesudo-resource-id).

### Terrafy a Resource Group

`aztfy resource-group [option] <resource group name>` terrafies a resource group and its including resources by its name.

### Terrafy a Customized Set of Resources

`aztfy query [option] <arg where predicate>` terrafies a set of resources (and its including resources with `--recursive`) by an Azure Resource Graph [`where` predicate](https://learn.microsoft.com/en-us/azure/data-explorer/kusto/query/whereoperator).

> 💡 Resource group mode is the same as running `aztfy query --recursive "resourceGroup =~ 'my-rg'"`, except it also add on the resource group itself.

`aztfy` depends on `azlist`, which uses ARG behind the scenes. `azlist` will first make an ARG call with the given where predicate, then if `--recursive` is specified, it will recursively call the "LIST" on the known child resource types. Since ARG only returns ARM tracked resources at this moment, but not for the RP proxy resources (e.g. subnet, network security rules, storage containers, etc). If you uses predicate like `type =~ "microsoft.network/virtualnetworks/subnets"`, it returns you nothing since subnet is not an ARM tracked resource.

To workaround above, you can query with a bigger scope (e.g. `type =~ "microsoft.network/virtualnetworks"`) in interactive mode, then manually remove the resources other than subnets.

### Terrafy a Predefined Set of Resources

`aztfy mapping-file [option] <resource mapping file>` terrafies a set of resources that is defined in the resource mapping file.

The format of the mapping file is defined below:

```json
{
    "<Azure resource id1>": {
        "resource_type" : "<terraform resource type>",
        "resource_name" : "<terraform resource name>",
        "resource_id"   : "<terraform resource id>"
    },
    "<Azure resource id2>": {
        "resource_type" : "<terraform resource type>",
        "resource_name" : "<terraform resource name>",
        "resource_id"   : "<terraform resource id>"
    },
    ...
}
```

Example:

```json
{
	"/subscriptions/0000/resourceGroups/aztfy-vmdisk": {
		"resource_id": "/subscriptions/0000/resourceGroups/aztfy-vmdisk",
		"resource_type": "azurerm_resource_group",
		"resource_name": "res-1"
	},
	"/subscriptions/0000/resourceGroups/aztfy-vmdisk/providers/Microsoft.Compute/disks/aztfy-test-test": {
		"resource_id": "/subscriptions/0000/resourceGroups/aztfy-vmdisk/providers/Microsoft.Compute/disks/aztfy-test-test",
		"resource_type": "azurerm_managed_disk",
		"resource_name": "res-2"
	},
	"/subscriptions/0000/resourceGroups/aztfy-vmdisk/providers/Microsoft.Compute/virtualMachines/aztfy-test-test": {
		"resource_id": "/subscriptions/0000/resourceGroups/aztfy-vmdisk/providers/Microsoft.Compute/virtualMachines/aztfy-test-test",
		"resource_type": "azurerm_linux_virtual_machine",
		"resource_name": "res-3"
	},
    ...
}
```

You can generate the mapping file in all other modes (i.e. `resource`, `resource-group`, `query`) by specifying the `--generate-mapping-file` option when running non-interactively, or press <kbd>s</kbd> when running interactively in the resource list stage. Also, each run of `aztfy` will generate the resource mapping file for you, to record what resources have been imported.

Of course, you are welcome to manually construct or edit the mapping file. Note that only the object value in the mapping file matters, while the key just plays as an identifier in this mode. 

### Interactive vs Non-Interactive

By default `aztfy` runs in interactive mode, whilst you can also run in non-interactive mode by adding the `--non-interactive`/`-n` option.

#### Interactive mode

In interactive mode, `aztfy` list all the resources resides in the specified resource group or customized set. For each resource, `aztfy` will try to recognize the corresponding Terraform resource type. If it finds one, the line will be prefixed by a 💡 as an indicator. Otherwise, user is expected to input the Terraform resource address in form of `<resource type>.<resource name>` (e.g. `azurerm_linux_virtual_machine.test`). Users can press `r` to see the possible resource type(s) for the selected resource.

In some cases, there are Azure resources that have no corresponding Terraform resources (e.g. due to lacks of Terraform support), or some resources might be created as a side effect of provisioning another resource (e.g. the OS Disk resource is created automatically when provisioning a VM). In these cases, you can skip these resources without typing anything.

After going through all the resources to be imported, users press `w` to instruct `aztfy` to proceed importing resources into Terraform state and generating the Terraform configuration.

#### Non-Interactive mode

In non-interactive mode, `aztfy` only imports the recognized resources, and skip the others. Users can further specify the `--continue`/`-k` option to make the tool continue even on hitting any import error.

### Remote Backend

By default `aztfy` uses local backend to store the state file. While it is also possible to use [remote backend](https://www.terraform.io/language/settings/backends), via the `--backend-type` and `--backend-config` options.

E.g. to use the [`azurerm` backend](https://www.terraform.io/language/settings/backends/azurerm#azurerm), users can invoke `aztfy` like following:

```shell
aztfy [subcommand] --backend-type=azurerm --backend-config=resource_group_name=<resource group name> --backend-config=storage_account_name=<account name> --backend-config=container_name=<container name> --backend-config=key=terraform.tfstate 
```

### Import Into Existing Local State

For local backend, `aztfy` will by default ensure the output directory is empty at the very begining. This is to avoid any conflicts happen for existing user files, including the terraform configuration, provider configuration, the state file, etc. As a result, `aztfy` generates a pretty new workspace for users.

One limitation of doing so is users can't import resources to existing state file via `aztfy`. To support this scenario, you can use the `--append` option. This option will make `aztfy` skip the empty guarantee for the output directory. If the output directory is empty, then it has no effect. Otherwise, it will ensure the provider setting (create a file for it if not exists). Then it proceeds the following steps.

This means if the output directory has an active Terraform workspace, i.e. there exists a state file, any resource imported by the `aztfy` will be imported into that state file. Especially, the file generated by `aztfy` in this case will be named differently than normal, where each file will has `.aztfy` suffix before the extension (e.g. `main.aztfy.tf`), to avoid potential file name conflicts. If you run `aztfy --append` multiple times, the generated config in `main.aztfy.tf` will be appended in each run.

## How it Works

`aztfy` leverage [`aztft`](https://github.com/magodo/aztft) to identify the Terraform resource type on its Azure resource ID. Then it runs `terraform import` under the hood to import each resource. Afterwards, it runs [`tfadd`](https://github.com/magodo/tfadd) to generate the Terraform template for each imported resource.

## Demo

[![asciicast](https://asciinema.org/a/xUnzCWXY0WYpC9nbBbMMhwb5L.svg)](https://asciinema.org/a/xUnzCWXY0WYpC9nbBbMMhwb5L)

## Limitation

There are several limitations causing `aztfy` can hardly generate reproducible Terraform configurations.

### AzureRM Provider Validation

When generating the Terraform configuration, not all properties of the resource are exported for different reasons.

One reason is because there are flexible cross-property constraints defined in the AzureRM Terraform provider. E.g. `property_a` conflits with `property_b`. This might due to the nature of the API, or might be due to some deprecation process of the provider (e.g. `property_a` is deprecated in favor of `property_b`, but kept for backwards compatibility). These constraints require some properties must be absent in the Terraform configuration, otherwise, the configuration is not a valid and will fail during `terraform validate`.

Another reason is that an Azure resource can be a property of its parent resource (e.g. `azurerm_subnet` can be its own resource, or be a property of `azurerm_virtual_network`). Per Terraform's best practice, users should only use one of the forms, not both. `aztfy` chooses to always generate all the resources, but omit the property in the parent resource that represents the child resource.

## Additional Resources

- [The aztfy Github Page](https://azure.github.io/aztfy): Everything about aztfy, including comparisons with other existing import solutions.
- [Kyle Ruddy's Blog about aztfy](https://www.kmruddy.com/2022/terrafy-existing-azure-resources/): A live use of `aztfy`, explaining the pros and cons.
- [aztft](https://github.com/magodo/aztft): A Go program and library for identifying the correct Terraform AzureRM provider resource type on the Azure resource id.
- [tfadd](https://github.com/magodo/tfadd): A Go program and library for generating Terraform configuration from Terraform state.
