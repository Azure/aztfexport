package cfgfile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateConfiguration(t *testing.T) {
	tests := []struct {
		name string
		ocfg Configuration
		ncfg Configuration
		k, v string
		err  string
	}{
		{
			name: "Invalid key",
			ocfg: Configuration{},
			k:    "nonexist",
			v:    "123",
			err:  `invalid key "nonexist"`,
		},
		{
			name: "Invalid value",
			ocfg: Configuration{},
			k:    "installation_id",
			v:    "123",
			err:  "unmarshalling the new configuration",
		},
		{
			name: "Valid update",
			ocfg: Configuration{},
			k:    "installation_id",
			v:    `"0000"`,
			ncfg: Configuration{InstallationId: "0000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ncfg, err := UpdateConfiguration(tt.ocfg, tt.k, tt.v)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.Equal(t, tt.ncfg, *ncfg)
		})
	}
}
