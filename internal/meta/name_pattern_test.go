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

func TestEnsureValidTFName(t *testing.T) {
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
		if got := ensureValidTFName(c.in); got != c.want {
			t.Errorf("ensureValidTFName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNameExpander(t *testing.T) {
	vm1 := resourceset.TFResource{
		AzureId: mustParseID(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/myRg/providers/Microsoft.Compute/virtualMachines/vm1"),
		TFType:  "azurerm_linux_virtual_machine",
	}
	vm2 := resourceset.TFResource{
		AzureId: mustParseID(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/myRg/providers/Microsoft.Compute/virtualMachines/vm2"),
		TFType:  "azurerm_linux_virtual_machine",
	}
	vnet := resourceset.TFResource{
		AzureId: mustParseID(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/myRg/providers/Microsoft.Network/virtualNetworks/vnet1"),
		TFType:  "azurerm_virtual_network",
	}

	t.Run("default-pattern", func(t *testing.T) {
		e := newNameExpander("res-")
		got := []string{e.Expand(vm1), e.Expand(vm2), e.Expand(vnet)}
		want := []string{"res-0", "res-1", "res-2"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("star-suffix", func(t *testing.T) {
		e := newNameExpander("res-*")
		got := []string{e.Expand(vm1), e.Expand(vm2)}
		want := []string{"res-0", "res-1"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("type-placeholder", func(t *testing.T) {
		e := newNameExpander("{type}_*")
		got := []string{e.Expand(vm1), e.Expand(vm2), e.Expand(vnet)}
		// Per-prefix counter restarts per distinct expanded prefix.
		want := []string{"virtual_machines_0", "virtual_machines_1", "virtual_networks_0"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("name-and-root_scope-placeholders", func(t *testing.T) {
		e := newNameExpander("{root_scope}_{name}_*")
		got := e.Expand(vm1)
		want := "my_rg_vm1_0"
		if got != want {
			t.Errorf("= %q, want %q", got, want)
		}
	})

	t.Run("rp-placeholder", func(t *testing.T) {
		e := newNameExpander("{rp}_{type}_*")
		got := e.Expand(vm1)
		want := "microsoft_compute_virtual_machines_0"
		if got != want {
			t.Errorf("= %q, want %q", got, want)
		}
	})

	t.Run("no-star-appends-index", func(t *testing.T) {
		e := newNameExpander("{type}")
		got := []string{e.Expand(vm1), e.Expand(vm2)}
		want := []string{"virtual_machines0", "virtual_machines1"}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("sanitizes-invalid-chars", func(t *testing.T) {
		e := newNameExpander("bad name!*")
		got := e.Expand(vm1)
		// Spaces and `!` become underscores; trailing underscore from `!` is kept (collapsed once with `*->0`).
		// We don't assert the exact collapsing rules but ensure the result is a valid identifier.
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
			if !ok {
				t.Errorf("invalid char %q in %q", r, got)
			}
		}
	})
}
