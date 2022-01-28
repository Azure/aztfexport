## Azure Terrafy

A tool to bring your existing Azure resources under the management of Terraform.

## Goal

Azure Terrafy imports the resources inside a resource group, which are supported by the Terraform provider, into the Terraform state, and generates the valid Terraform configuration. Both the Terraform state and configuration should be consistent with the resources' remote state, i.e., `terraform plan` shows no diff. The user then is able to use Terraform to manage these resources.

## Install

### From Release

Precompiled binaries for Windows, OS X, Linux are available at [Releases](https://github.com/Azure/aztfy/releases).

Note: The release is in the format of `.tar.gz`, Windows users might want to have [7zip](https://www.7-zip.org/download.html) installed to extract the files.

### From Go toolchain

```bash
go install github.com/Azure/aztfy@latest
```

## Usage

Follow the [authentication guide](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs#authenticating-to-azure) from the Terraform AzureRM provider to authenticate to Azure. The simplist way is to [install and login via the Azure CLI](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/guides/azure_cli).

Then you can go ahead and run `aztfy`:

```shell
aztfy [option] <resource group name>

  -k    Whether continue on import error (quiet mode only)
  -m string
        Specify the resource mapping file
  -o string
        Specify output dir. Default is a dir under the user cache dir, which is named after the resource group name
  -q    Quiet mode
  -v    Print version
```

The tool will list all the resources resides in the specified resource group.

For each resource, `aztfy` will ask the user to input the Terraform resource type for each Azure resource (e.g. `azurerm_linux_virtual_machine`). Users can press `r` to see the possible resource type for the selected import item, though this is not guaranteed to be 100% accurate. In case there is exactly one resource type match for the import item, that resource type will be automatically be filled in the text input for the users, with a 💡 line prefix as an indication.

In some cases, there are Azure resources that have no corresponding Terraform resource (e.g. due to lacks of Terraform support), or some resource might be created as a side effect of provisioning another resource (e.g. the Disk resource is created automatically when provisioning a VM). In these cases, you can skip these resources without typing anything.

> 💡 Option `-m` can be used to specify a resource mapping file, either constructed manually or from other runs of `aztfy` (generated in the output directory with name: _.aztfyResourceMapping.json_).

After going through all the resources to be imported, users press `w` to proceed.`aztfy` will run `terraform import` under the hood to import each resource. Then it will run `terraform add -from-state` to generate the Terraform template for each imported resource. Whereas there are kinds of [limitations](https://github.com/apparentlymart/terrafy/blob/main/docs/quirks.md) causing the output of `terraform add` to be an invalid Terraform template in most cases. `aztfy` will leverage extra knowledge from the provider (which is generated from the provider codebase) to further manipulate the template, to make it pass the Terraform validations against the provider.

As the last step, `aztfy` will leverage the ARM template to inject dependencies between each resource. This makes the generated Terraform template to be useful.

### Batch Import Mode

`aztfy` also supports to be run non-interactively by setting the `-q` option. In this mode, a resource mapping file is mandatory to be specified via option `-m`, with the following content:

```json
{
    "<azure resource id1>": "<terraform resource type1>",
    "<azure resource id2>": "<terraform resource type2>",
    ...
}
```

Example:

```json
{
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/virtualNetworks/example-network": "azurerm_virtual_network",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Compute/virtualMachines/example-machine": "azurerm_linux_virtual_machine",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/networkInterfaces/example-nic": "azurerm_network_interface",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/networkInterfaces/example-nic1": "azurerm_network_interface",
  "/subscriptions/0-0-0-0/resourceGroups/tfy-vm/providers/Microsoft.Network/virtualNetworks/example-network/subnets/internal": "azurerm_subnet"
}
```

Then the tool will import each specified resource in the mapping file (if exists) and skip the others. Then generate the Terraform configuration with dependencies resolved.

In the batch import mode, users can further specify the `-k` option to make the tool continue even on hittng import error on any resource.

## Demo

[![asciicast](https://asciinema.org/a/jZU8De8Kok98zBVyMTtktDQnO.svg)](https://asciinema.org/a/jZU8De8Kok98zBVyMTtktDQnO)

## Limitation

Some Azure resources are modeled differently in AzureRM provider, which means there might be N:M mapping between the Azure resources and the Terraform resources.

For example, the `azurerm_lb_backend_address_pool_address` is actually a property of `azurerm_lb_backend_address_pool`, whilst in the AzureRM provider, it has its own resource and a synthetic resource ID as `/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/group1/providers/Microsoft.Network/loadBalancers/loadBalancer1/backendAddressPools/backendAddressPool1/addresses/address1`.

Another popular case is that in the AzureRM provider, there are a bunch of "association" resources, e.g. the `azurerm_network_interface_security_group_association`. These "association" resources represent the association relationship between two Terraform resources (in this case they are `azurerm_network_interface` and `azurerm_network_security_group`). They also have some synthetic resource ID, e.g. `/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/mygroup1/providers/microsoft.network/networkInterfaces/example|/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/group1/providers/Microsoft.Network/networkSecurityGroups/group1`.

Currently, this tool only works on the assumption that there is 1:1 mapping between Azure resources and the Terraform resources.

## How to develop with vscode

### vs pre requiries

1. Install Go extension from "Go Team at Google"

1. Install dependencies when ask in the editor.

1. Build without optimision  
    go build -gcflags=all="-N -l"
   (To run, in context of the folder)

1. Add some code in the main.go to stop the init.

```
var reader = bufio.NewReader(os.Stdin)
os.Setenv("AZTFY_DEBUG", "true")
os.Setenv("AZTFY_MOCK_CLIENT", "false")
os.Setenv("AZTFY_LOGFILE", "aztfylogs.log")

log.Println("Main hitted")

input, _ := reader.ReadString('\n')
value := strings.TrimSpace(input)
fmt.Printf("input: %v", value)
```

2. Run app

```
./aztfy rg-my-demo
```

1. Get pid of the app

   - Linux : pgrep aztfy
   - Windows : Task manager / tab detail

2. Update launch setting processId with pid
   (Sample in folder .vscode\launch.json)

3. launch debug session

4. Press enter
