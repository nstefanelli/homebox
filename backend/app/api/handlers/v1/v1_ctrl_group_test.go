package v1

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func TestMapInvitationAcceptError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "member conflict", err: repo.ErrAlreadyGroupMember, status: http.StatusConflict},
		{name: "expired invitation", err: repo.ErrInvitationExpired, status: http.StatusBadRequest},
		{name: "exhausted invitation", err: repo.ErrInvitationExhausted, status: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapInvitationAcceptError(tt.err)
			var requestErr *validate.RequestError
			require.ErrorAs(t, got, &requestErr)
			assert.Equal(t, tt.status, requestErr.Status)
			assert.ErrorIs(t, got, tt.err)
		})
	}
}

func TestMapInvitationAcceptErrorPreservesUnexpectedErrors(t *testing.T) {
	t.Parallel()

	unexpected := errors.New("database unavailable")
	assert.ErrorIs(t, mapInvitationAcceptError(unexpected), unexpected)
}
