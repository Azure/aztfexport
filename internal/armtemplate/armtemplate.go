package armtemplate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/aztfy/internal/client"
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

func ParseResourceId(id string) (*ResourceId, error) {
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
		t = append(t, segs[i])
		n = append(n, segs[i+1])
	}

	return &ResourceId{
		Type: strings.Join(t, "/"),
		Name: strings.Join(n, "/"),
	}, nil
}

func ParseResourceIdFromCallExpr(expr string) (*ResourceId, error) {
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

// ID converts the ARM ResourceId to its ARM resource ID literal, based on the specified subscription id and resource
// group name.
func (res ResourceId) ID(sub, rg string) string {
	typeSegs := strings.Split(res.Type, "/")
	nameSegs := strings.Split(res.Name, "/")

	segs := []string{"/subscriptions", sub, "resourceGroups", rg}
	if len(typeSegs) != 1 {
		if len(typeSegs)-1 != len(nameSegs) {
			panic(fmt.Sprintf("The resource of type %q and name %q is not a valid identifier", res.Type, res.Name))
		}
		segs = append(segs, "providers", typeSegs[0])
		for i := 0; i < len(nameSegs); i++ {
			segs = append(segs, typeSegs[i+1])
			segs = append(segs, nameSegs[i])
		}
	}
	return strings.Join(segs, "/")
}

type ResourceIds []ResourceId

func (resids *ResourceIds) UnmarshalJSON(b []byte) error {
	var residExprs []string
	if err := json.Unmarshal(b, &residExprs); err != nil {
		return err
	}

	var ids ResourceIds
	for _, residExpr := range residExprs {
		id, err := ParseResourceIdFromCallExpr(residExpr)
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
	// The Id is representing the TF resource ID.
	Id         string
	Properties interface{}
	// The IDs in the DependsOn are TF resource IDs.
	DependsOn []string
}

func (tpl FQTemplate) DependencyInfo() (map[string][]string, error) {
	s := map[string][]string{}
	for _, res := range tpl.Resources {
		if len(res.DependsOn) == 0 {
			rgid, _ := ResourceGroupId.ProviderId(tpl.subId, tpl.rg, nil)
			s[res.Id] = []string{rgid}
			continue
		}
		s[res.Id] = res.DependsOn
	}
	return s, nil
}

// Qualify converts the raw ARM template to a fully qualified form in turns of the resource ids, where the ids are converted from the
// ARM resource id expression to the corresponding TF resource id form.
func (tpl Template) Qualify(subId, rg string, b *client.ClientBuilder) (*FQTemplate, error) {
	fqtpl := FQTemplate{
		subId: subId,
		rg:    rg,
	}
	for _, res := range tpl.Resources {
		id, err := res.ResourceId.ProviderId(subId, rg, b)
		if err != nil {
			return nil, err
		}
		fqres := FQResource{
			Id:         id,
			Properties: res.Properties,
		}
		for _, d := range res.DependsOn {
			id, err := d.ProviderId(subId, rg, b)
			if err != nil {
				return nil, err
			}
			fqres.DependsOn = append(fqres.DependsOn, id)
		}
		fqtpl.Resources = append(fqtpl.Resources, fqres)
	}
	return &fqtpl, nil
}
