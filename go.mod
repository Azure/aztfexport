module github.com/Azure/aztfexport

go 1.23.0

toolchain go1.24.1

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.9.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault v1.5.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph v0.9.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0
	github.com/charmbracelet/bubbles v0.14.0
	github.com/charmbracelet/bubbletea v0.22.1
	github.com/charmbracelet/lipgloss v0.5.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/hashicorp/go-hclog v1.6.3
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/go-version v1.7.0
	github.com/hashicorp/hc-install v0.9.2
	github.com/hashicorp/hcl/v2 v2.23.0
	github.com/hashicorp/terraform-config-inspect v0.0.0-20221020162138-81db043ad408
	github.com/hashicorp/terraform-exec v0.23.0
	github.com/hashicorp/terraform-json v0.24.0
	github.com/hexops/gotextdiff v1.0.3
	github.com/magodo/armid v0.0.0-20240524082432-7ce06ae46c33
	github.com/magodo/azlist v0.0.0-20250429005249-2b51aca0f1e1
	github.com/magodo/aztft v0.3.1-0.20250429031349-753141d53de2
	github.com/magodo/slog2hclog v0.0.0-20240614031327-090ebd72a033
	github.com/magodo/spinner v0.0.0-20240524082745-3a2305db1bdc
	github.com/magodo/terraform-client-go v0.0.0-20240804032252-6d93a97fabb2
	github.com/magodo/textinput v0.0.0-20210913072708-7d24f2b4b0c0
	github.com/magodo/tfadd v0.10.1-0.20250429010038-dfc35ae7ffb2
	github.com/magodo/tfmerge v0.0.0-20221214062955-f52e46d03402
	github.com/magodo/tfstate v0.0.0-20241016043929-2c95177bf0e6
	github.com/magodo/workerpool v0.0.0-20240524082508-11838001bc35
	github.com/microsoft/ApplicationInsights-Go v0.4.4
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/muesli/reflow v0.3.0
	github.com/pkg/profile v1.7.0
	github.com/stretchr/testify v1.10.0
	github.com/tidwall/gjson v1.14.2
	github.com/tidwall/sjson v1.2.5
	github.com/urfave/cli/v2 v2.27.6
	github.com/zclconf/go-cty v1.16.2
	software.sslmate.com/src/go-pkcs12 v0.4.0
)

require (
	code.cloudfoundry.org/clock v0.0.0-20180518195852-02e53af36e6c // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/alertsmanagement/armalertsmanagement v0.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/apimanagement/armapimanagement/v3 v3.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3 v3.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/applicationinsights/armapplicationinsights v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appplatform/armappplatform/v2 v2.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4 v4.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/automation/armautomation v0.9.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/botservice/armbotservice v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cdn/armcdn/v2 v2.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cognitiveservices/armcognitiveservices v1.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/costmanagement/armcostmanagement/v2 v2.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/datafactory/armdatafactory/v7 v7.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dataprotection/armdataprotection/v3 v3.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/datashare/armdatashare v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2 v2.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/devtestlabs/armdevtestlabs v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/digitaltwins/armdigitaltwins v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/domainservices/armdomainservices v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/frontdoor/armfrontdoor v1.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/hdinsight/armhdinsight v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/hybridkubernetes/armhybridkubernetes v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/iothub/armiothub v1.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/kusto/armkusto/v2 v2.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/logic/armlogic v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/machinelearning/armmachinelearning/v4 v4.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/netapp/armnetapp/v7 v7.5.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6 v6.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/operationalinsights/armoperationalinsights v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/paloaltonetworksngfw/armpanngfw v1.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/recoveryservices/armrecoveryservicesbackup/v4 v4.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/recoveryservices/armrecoveryservicessiterecovery/v2 v2.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeploymentscripts/v2 v2.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2 v2.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/securityinsights/armsecurityinsights/v2 v2.0.0-beta.4 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/sql/armsql v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.8.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagecache/armstoragecache/v4 v4.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagemover/armstoragemover/v2 v2.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storagepool/armstoragepool v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/streamanalytics/armstreamanalytics v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/synapse/armsynapse v0.8.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/timeseriesinsights/armtimeseriesinsights v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/webpubsub/armwebpubsub v1.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/workloads/armworkloads v1.1.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.4.2 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/apparentlymart/go-textseg/v15 v15.0.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/charmbracelet/harmonica v0.2.0 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/felixge/fgprof v0.9.3 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20211214055906-6f57359322fd // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-plugin v1.6.0 // indirect
	github.com/hashicorp/hcl v0.0.0-20170504190234-a4b07c25de5f // indirect
	github.com/hashicorp/terraform-plugin-go v0.23.0 // indirect
	github.com/hashicorp/yamux v0.1.1 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magodo/tfpluginschema v0.0.0-20240902090353-0525d7d8c1c2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/muesli/ansi v0.0.0-20211018074035-2e021307bc4b // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.11.1-0.20220212125758-44cd13922739 // indirect
	github.com/oklog/run v1.0.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sahilm/fuzzy v0.1.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/term v0.31.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	golang.org/x/tools v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250324211829-b45e905df463 // indirect
	google.golang.org/grpc v1.73.0-dev.0.20250513095349-b89909b7bd0d // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
