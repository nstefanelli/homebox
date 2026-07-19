package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

func TestResolveDemoPassword(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		configured string
		want       string
		wantError  bool
	}{
		{
			name: "development uses public default",
			mode: config.ModeDevelopment,
			want: demoPasswordDefault,
		},
		{
			name:       "development preserves a short fixture password",
			mode:       config.ModeDevelopment,
			configured: "short",
			want:       "short",
		},
		{
			name:      "production rejects an unset password",
			mode:      config.ModeProduction,
			wantError: true,
		},
		{
			name:       "production rejects a short password",
			mode:       config.ModeProduction,
			configured: strings.Repeat("a", demoPasswordMinLength-1),
			wantError:  true,
		},
		{
			name:       "production accepts the minimum length",
			mode:       config.ModeProduction,
			configured: strings.Repeat("a", demoPasswordMinLength),
			want:       strings.Repeat("a", demoPasswordMinLength),
		},
		{
			name:       "unknown mode does not restore the development default",
			mode:       "prodution",
			configured: "",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDemoPassword(tt.mode, tt.configured)
			if tt.wantError {
				require.Error(t, err)
				assert.Empty(t, got)
				assert.Contains(t, err.Error(), demoPasswordEnv)
				assert.Contains(t, err.Error(), "12")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateDemoConfig(t *testing.T) {
	t.Run("disabled demo does not require a password", func(t *testing.T) {
		t.Setenv(demoPasswordEnv, "")
		require.NoError(t, validateDemoConfig(&config.Config{
			Mode: config.ModeProduction,
			Demo: false,
		}))
	})

	t.Run("production demo requires a password", func(t *testing.T) {
		t.Setenv(demoPasswordEnv, "")
		require.Error(t, validateDemoConfig(&config.Config{
			Mode: config.ModeProduction,
			Demo: true,
		}))
	})

	t.Run("production demo accepts an explicit strong password", func(t *testing.T) {
		t.Setenv(demoPasswordEnv, strings.Repeat("a", demoPasswordMinLength))
		require.NoError(t, validateDemoConfig(&config.Config{
			Mode: config.ModeProduction,
			Demo: true,
		}))
	})
}

func TestRunRejectsProductionDemoWithoutPassword(t *testing.T) {
	t.Setenv(demoPasswordEnv, "")

	err := run(&config.Config{
		Mode: config.ModeProduction,
		Demo: true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), demoPasswordEnv)
}
