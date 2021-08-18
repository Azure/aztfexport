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
	DependsOn Dependencies `json:"dependsOn,omitempty"`
}

type ResourceId struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

var ResourceGroupId = ResourceId{}

func NewResourceId(id string) (*ResourceId, error) {
	id = strings.TrimPrefix(id, "/")
	id = strings.TrimSuffix(id, "/")
	segs := strings.Split(id, "/")
	if len(segs)%2 != 0 {
		return nil, fmt.Errorf("invalid resource id format of %q: amount of segments is not even", id)
	}
	// ==4: resource group
	// >=8: general resources resides in a resource group
	if len(segs) != 4 && len(segs) < 8 {
		return nil, fmt.Errorf("invalid resource id format of %q: amount of segments is too small", id)
	}
	if segs[0] != "subscriptions" {
		return nil, fmt.Errorf("invalid resource id format of %q: the 1st segment is not subscriptions", id)
	}
	segs = segs[2:]
	if !strings.EqualFold(segs[0], "resourcegroups") {
		return nil, fmt.Errorf("invalid resource id format of %q: the 2nd segment is not resourcegroups (case insensitive)", id)
	}
	segs = segs[2:]

	if len(segs) == 0 {
		return &ResourceGroupId, nil
	}

	if segs[0] != "providers" {
		return nil, fmt.Errorf("invalid resource id format of %q: the 3rd segment is not providers", id)
	}
	providerName := segs[1]
	segs = segs[2:]

	t := []string{providerName}
	n := []string{}

	for i := 0; i < len(segs); i += 2 {
		t = append(t, segs[0])
		n = append(n, segs[1])
	}

	return &ResourceId{
		Type: strings.Join(t, "/"),
		Name: strings.Join(n, "/"),
	}, nil
}

// ID returns the azure resource id
func (res ResourceId) ID(sub, rg string) string {
	typeSegs := strings.Split(res.Type, "/")
	nameSegs := strings.Split(res.Name, "/")

	out := []string{"subscriptions", sub, "resourceGroups", rg}
	if len(typeSegs) != 0 {
		if len(typeSegs)-1 != len(nameSegs) {
			panic(fmt.Sprintf("The resource of type %q and name %q is not a valid identifier", res.Type, res.Name))
		}
		out = append(out, typeSegs[0])
		for i := 0; i < len(nameSegs); i++ {
			out = append(out, typeSegs[i+1])
			out = append(out, nameSegs[i])
		}
	}
	return strings.Join(out, "/")
}

type Dependencies []ResourceId

func (deps *Dependencies) UnmarshalJSON(b []byte) error {
	var dependenciesRaw []string
	if err := json.Unmarshal(b, &dependenciesRaw); err != nil {
		return err
	}

	for _, dep := range dependenciesRaw {
		matches := regexp.MustCompile(`^\[resourceId\(([^,]+), (.+)\)]$`).FindAllStringSubmatch(dep, 1)
		if len(matches) == 0 {
			panic(fmt.Sprintf("the dependency %q is not valid (no match)", dep))
		}
		m := matches[0]
		if len(m) != 3 {
			panic(fmt.Sprintf("the dependency %q is not valid (the matched one has invalid form)", dep))
		}

		tlit, nlit := m[1], m[2]

		t := strings.Trim(tlit, "' ")

		var names []string
		for _, seg := range strings.Split(nlit, ",") {
			names = append(names, strings.Trim(seg, "' "))
		}
		n := strings.Join(names, "/")

		*deps = append(*deps, ResourceId{
			Type: t,
			Name: n,
		})
	}
	return nil
}

type DependencyInfo map[ResourceId][]ResourceId

func (tpl Template) DependencyInfo(rgName string) DependencyInfo {
	s := map[ResourceId][]ResourceId{}
	for _, res := range tpl.Resources {
		if len(res.DependsOn) == 0 {
			s[res.ResourceId] = []ResourceId{ResourceGroupId}
			continue
		}

		s[res.ResourceId] = res.DependsOn
	}
	return s
}
