package v1

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func TestQueryIntOrNegativeOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "absent", raw: "", want: -1},
		{name: "valid", raw: "42", want: 42},
		{name: "negative", raw: "-2", want: -2},
		{name: "malformed", raw: "many", want: 0},
		{name: "overflow", raw: "999999999999999999999999999999999999", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, queryIntOrNegativeOne(tt.raw))
		})
	}
}

func TestPaginationRequestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		status int
	}{
		{
			name:   "invalid pagination",
			err:    fmt.Errorf("query failed: %w", repo.ErrInvalidPagination),
			status: http.StatusBadRequest,
		},
		{
			name:   "unexpected repository error",
			err:    errors.New("database unavailable"),
			status: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := paginationRequestError(tt.err)
			var requestErr *validate.RequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, tt.status, requestErr.Status)
			assert.ErrorIs(t, err, tt.err)
		})
	}
}
