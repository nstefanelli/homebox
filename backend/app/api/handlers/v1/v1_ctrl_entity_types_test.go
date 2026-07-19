package v1

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func TestValidateEntityTypeFlags(t *testing.T) {
	t.Parallel()

	for _, flags := range []struct {
		name        string
		isLocation  bool
		isContainer bool
	}{
		{name: "item"},
		{name: "location", isLocation: true},
		{name: "container", isLocation: true, isContainer: true},
	} {
		t.Run(flags.name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateEntityTypeFlags(flags.isLocation, flags.isContainer))
		})
	}

	err := validateEntityTypeFlags(false, true)
	require.ErrorIs(t, err, repo.ErrContainerRequiresLocation)
	var requestErr *validate.RequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, http.StatusBadRequest, requestErr.Status)
}
