package meta

import (
	"testing"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/magodo/armid"
)

func mustParseID(t *testing.T, s string) armid.ResourceId {
	t.Helper()
	id, err := armid.ParseResourceId(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return id
}

func TestSnakeCase(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"virtualMachines", "virtual_machines"},
		{"HTTPServer", "http_server"},
		{"Microsoft.Compute", "microsoft_compute"},
		{"my-vm.01", "my_vm_01"},
		{"azurerm_virtual_machine", "azurerm_virtual_machine"},
		{"FooBARBaz", "foo_bar_baz"},
		{"--__weird__--", "weird"},
	}
	for _, c := range cases {
		if got := snakeCase(c.in); got != c.want {
			t.Errorf("snakeCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToTFName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "res"},
		{"foo", "foo"},
		{"1foo", "_1foo"},
		{"foo.bar", "foo_bar"},
		{"foo-bar_baz0", "foo-bar_baz0"},
	}
	for _, c := range cases {
		if got := toTFName(c.in); got != c.want {
			t.Errorf("ensureValidTFName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNameExpander(t *testing.T) {
	vm1 := resourceset.TFResource{
		AzureId: mustParseID(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/myRg/providers/Microsoft.Compute/virtualMachines/vmone"),
		TFType:  "azurerm_linux_virtual_machine",
	}
	vm2 := resourceset.TFResource{
		AzureId: mustParseID(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/myRg/providers/Microsoft.Compute/virtualMachines/vmtwo"),
		TFType:  "azurerm_linux_virtual_machine",
	}
	vnet := resourceset.TFResource{
		AzureId: mustParseID(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/myRg/providers/Microsoft.Network/virtualNetworks/myvnet"),
		TFType:  "azurerm_virtual_network",
	}

	t.Run("default-pattern", func(t *testing.T) {
		// The `*` is implicitly appended.
		e := newNameExpander("res")
		got := []string{e.Expand(vm1), e.Expand(vm2), e.Expand(vnet)}
		want := []string{"res", "res2", "res"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("star-single-resource", func(t *testing.T) {
		e := newNameExpander("res*")
		got := e.Expand(vm1)
		want := "res"
		if got != want {
			t.Errorf("= %q, want %q", got, want)
		}
	})

	t.Run("star-infix", func(t *testing.T) {
		e := newNameExpander("pre*post")
		got := []string{e.Expand(vm1), e.Expand(vm2)}
		want := []string{"prepost", "pre2post"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("plus-always-index", func(t *testing.T) {
		e := newNameExpander("res-+")
		got := []string{e.Expand(vm1), e.Expand(vm2), e.Expand(vnet)}
		want := []string{"res-1", "res-2", "res-1"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("plus-infix", func(t *testing.T) {
		e := newNameExpander("pre_+_post")
		got := []string{e.Expand(vm1), e.Expand(vm2)}
		want := []string{"pre_1_post", "pre_2_post"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("type-placeholder", func(t *testing.T) {
		e := newNameExpander("{type}")
		got := []string{e.Expand(vm1), e.Expand(vm2), e.Expand(vnet)}
		// Per-prefix counter restarts per distinct expanded prefix.
		want := []string{"virtual_machines", "virtual_machines2", "virtual_networks"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("name-and-root_scope-placeholders", func(t *testing.T) {
		e := newNameExpander("{root_scope}_{name}")
		got := e.Expand(vm1)
		want := "my_rg_vmone"
		if got != want {
			t.Errorf("= %q, want %q", got, want)
		}
	})

	t.Run("rp-placeholder", func(t *testing.T) {
		e := newNameExpander("{rp}_{type}")
		got := e.Expand(vm1)
		want := "microsoft_compute_virtual_machines"
		if got != want {
			t.Errorf("= %q, want %q", got, want)
		}
	})

	t.Run("sanitizes-invalid-chars", func(t *testing.T) {
		e := newNameExpander("bad name!*")
		got := e.Expand(vm1)
		// Spaces and `!` become underscores.
		// We don't assert the exact collapsing rules but ensure the result is a valid identifier.
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
			if !ok {
				t.Errorf("invalid char %q in %q", r, got)
			}
		}
	})
}
