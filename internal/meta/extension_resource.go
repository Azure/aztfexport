package meta

import (
	"strings"

	"github.com/magodo/azlist/azlist"
)

// Supported extension resource types
const (
	ExtensionResourceTypeRoleAssignments    = "role-assignments"
	ExtensionResourceTypeLocks              = "locks"
	ExtensionResourceTypeDiagnosticSettings = "diagnostics-settings"
)

// SupportedExtensionResourceTypes is the list of supported extension resource types.
var SupportedExtensionResourceTypes = []string{
	ExtensionResourceTypeRoleAssignments,
	ExtensionResourceTypeLocks,
	ExtensionResourceTypeDiagnosticSettings,
}

type extBuilder struct {
	includeExtensions []string
}

func (b extBuilder) Build() []azlist.ExtensionResource {
	var el []azlist.ExtensionResource
	for _, ext := range b.includeExtensions {
		switch ext {
		case ExtensionResourceTypeRoleAssignments:
			el = append(el, azlist.ExtensionResource{
				Type: "Microsoft.Authorization/roleAssignments",
				Filter: func(res, extensionRes map[string]any) bool {
					idRaw, ok := res["id"]
					if !ok {
						return false
					}
					id := idRaw.(string)

					propsRaw, ok := extensionRes["properties"]
					if !ok {
						return false
					}
					scopeRaw, ok := propsRaw.(map[string]any)["scope"]
					if !ok {
						return false
					}
					scope := scopeRaw.(string)

					return strings.EqualFold(id, scope)
				},
			})
		case ExtensionResourceTypeLocks:
			el = append(el, azlist.ExtensionResource{
				Type: "Microsoft.Authorization/locks",
				Filter: func(res, extensionRes map[string]any) bool {
					idRaw, ok := res["id"]
					if !ok {
						return false
					}
					id := idRaw.(string)

					lockIdRaw, ok := extensionRes["id"]
					if !ok {
						return false
					}
					lockId := lockIdRaw.(string)

					// A management lock's id is in form of "<scope>/providers/Microsoft.Authorization/locks/<name>".
					// Derive the scope that the lock applies to and only keep the lock if that scope is
					// one of the exported resources (i.e. matches the parent resource being listed).
					idx := strings.Index(strings.ToLower(lockId), "/providers/microsoft.authorization/locks/")
					if idx == -1 {
						return false
					}
					scope := lockId[:idx]

					return strings.EqualFold(id, scope)
				},
			})
		case ExtensionResourceTypeDiagnosticSettings:
			el = append(el, azlist.ExtensionResource{
				Type: "Microsoft.Insights/diagnosticSettings",
			})
		}
	}

	return el
}
