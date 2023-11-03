package meta

import (
	"strings"

	"github.com/magodo/azlist/azlist"
)

type extBuilder struct {
	includeRoleAssignment bool
}

func (b extBuilder) Build() []azlist.ExtensionResource {
	var el []azlist.ExtensionResource
	if b.includeRoleAssignment {
		el = append(el, azlist.ExtensionResource{
			Type: "Microsoft.Authorization/roleAssignments",
			Filter: func(res, extensionRes map[string]interface{}) bool {
				idRaw, ok := res["id"]
				if !ok {
					return false
				}
				id := idRaw.(string)

				propsRaw, ok := extensionRes["properties"]
				if !ok {
					return false
				}
				scopeRaw, ok := propsRaw.(map[string]interface{})["scope"]
				if !ok {
					return false
				}
				scope := scopeRaw.(string)

				return strings.EqualFold(id, scope)
			},
		})
	}

	return el
}
