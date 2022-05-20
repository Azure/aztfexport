package mapping

import (
	"regexp"
	"strings"
)

var AzureIdPatternToResourcesMapping = map[*regexp.Regexp][]string{}

func init() {
	azureIdToResourcesMapping := map[string][]string{}
	for k, v := range ProviderResourceMapping {
		resources, ok := azureIdToResourcesMapping[v]
		if !ok {
			resources = []string{}
		}
		resources = append(resources, k)
		azureIdToResourcesMapping[v] = resources
	}
	for path, resources := range azureIdToResourcesMapping {
		p := regexp.MustCompile("^" + strings.ReplaceAll(path, "{}", "[^/]+") + "$")
		AzureIdPatternToResourcesMapping[p] = resources
	}
}
