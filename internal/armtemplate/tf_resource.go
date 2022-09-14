package armtemplate

// Key is the TF Resource Id
type TFResources map[string]TFResource

type TFResource struct {
	AzureId    string
	TFId       string
	TFType     string
	Properties interface{}
	DependsOn  []string // TF resource IDs
}