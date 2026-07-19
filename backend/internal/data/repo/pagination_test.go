package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePagination(t *testing.T) {
	t.Parallel()

	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name         string
		page         int
		pageSize     int
		wantPaginate bool
		wantErr      bool
	}{
		{name: "omitted", page: -1, pageSize: -1, wantPaginate: false},
		{name: "first page", page: 1, pageSize: 1, wantPaginate: true},
		{name: "maximum page size", page: 2, pageSize: MaxPageSize, wantPaginate: true},
		{name: "largest safe offset", page: maxInt, pageSize: 1, wantPaginate: true},
		{name: "missing page", page: -1, pageSize: 10, wantErr: true},
		{name: "missing page size", page: 1, pageSize: -1, wantErr: true},
		{name: "zero page", page: 0, pageSize: 10, wantErr: true},
		{name: "zero page size", page: 1, pageSize: 0, wantErr: true},
		{name: "negative values", page: -2, pageSize: -2, wantErr: true},
		{name: "oversized page size", page: 1, pageSize: MaxPageSize + 1, wantErr: true},
		{name: "offset overflow", page: maxInt, pageSize: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			paginate, err := validatePagination(tt.page, tt.pageSize)
			assert.Equal(t, tt.wantPaginate, paginate)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidPagination)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCalculateOffset(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, calculateOffset(1, 25))
	assert.Equal(t, 50, calculateOffset(3, 25))
}
