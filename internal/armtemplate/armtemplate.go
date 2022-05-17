package armtemplate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type Template struct {
	Resources []Resource `json:"resources"`
}

type Resource struct {
	ResourceId
	Properties interface{} `json:"properties,omitempty"`
	DependsOn  ResourceIds `json:"dependsOn,omitempty"`
}

type ResourceId struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

var ResourceGroupId = ResourceId{}

func NewResourceIdFromCallExpr(expr string) (*ResourceId, error) {
	matches := regexp.MustCompile(`^\[resourceId\(([^,]+), (.+)\)]$`).FindAllStringSubmatch(expr, 1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("the resourceId call expression %q is not valid (no match)", expr)
	}
	m := matches[0]
	if len(m) != 3 {
		return nil, fmt.Errorf("the resourceId call expression %q is not valid (the matched one has invalid form)", expr)
	}

	tlit, nlit := m[1], m[2]

	t := strings.Trim(tlit, "' ")

	var names []string
	for _, seg := range strings.Split(nlit, ",") {
		names = append(names, strings.Trim(seg, "' "))
	}
	n := strings.Join(names, "/")

	return &ResourceId{
		Type: t,
		Name: n,
	}, nil
}

// ID returns the azure resource id
func (res ResourceId) ID(sub, rg string) string {
	typeSegs := strings.Split(res.Type, "/")
	nameSegs := strings.Split(res.Name, "/")

	out := []string{"/subscriptions", sub, "resourceGroups", rg}
	if len(typeSegs) != 1 {
		if len(typeSegs)-1 != len(nameSegs) {
			panic(fmt.Sprintf("The resource of type %q and name %q is not a valid identifier", res.Type, res.Name))
		}
		out = append(out, "providers", typeSegs[0])
		for i := 0; i < len(nameSegs); i++ {
			out = append(out, typeSegs[i+1])
			out = append(out, nameSegs[i])
		}
	}
	return strings.Join(out, "/")
}

type ResourceIds []ResourceId

func (resids *ResourceIds) UnmarshalJSON(b []byte) error {
	var residExprs []string
	if err := json.Unmarshal(b, &residExprs); err != nil {
		return err
	}

	var ids ResourceIds
	for _, residExpr := range residExprs {
		id, err := NewResourceIdFromCallExpr(residExpr)
		if err != nil {
			return err
		}
		ids = append(ids, *id)
	}
	*resids = ids
	return nil
}

type FQTemplate struct {
	subId     string
	rg        string
	Resources []FQResource
}

type FQResource struct {
	Id         string
	Properties interface{}
	DependsOn  []string
}

func (tpl FQTemplate) DependencyInfo() map[string][]string {
	s := map[string][]string{}
	for _, res := range tpl.Resources {
		if len(res.DependsOn) == 0 {
			s[res.Id] = []string{ResourceGroupId.ID(tpl.subId, tpl.rg)}
			continue
		}
		s[res.Id] = res.DependsOn
	}
	return s
}

func (tpl Template) Qualify(subId, rg string) FQTemplate {
	fqtpl := FQTemplate{
		subId: subId,
		rg:    rg,
	}
	for _, res := range tpl.Resources {
		fqres := FQResource{
			Id:         res.ResourceId.ID(subId, rg),
			Properties: res.Properties,
		}
		for _, d := range res.DependsOn {
			fqres.DependsOn = append(fqres.DependsOn, d.ID(subId, rg))
		}
		fqtpl.Resources = append(fqtpl.Resources, fqres)
	}
	return fqtpl
}
