package repo

import (
	"errors"
	"fmt"
)

const MaxPageSize = 500

var ErrInvalidPagination = errors.New("invalid pagination")

type PaginationResult[T any] struct {
	Page                   int     `json:"page"`
	PageSize               int     `json:"pageSize"`
	Total                  int     `json:"total"`
	Items                  []T     `json:"items"`
	FilteredInventoryValue float64 `json:"-"`
}

// EntityListResult is the response type for paginated entity queries,
// extending PaginationResult with a total price summary.
type EntityListResult struct {
	PaginationResult[EntitySummary]
	TotalPrice float64 `json:"totalPrice"`
}

func calculateOffset(page, pageSize int) int {
	return (page - 1) * pageSize
}

func validatePagination(page, pageSize int) (bool, error) {
	if page == -1 && pageSize == -1 {
		return false, nil
	}
	if page < 1 || pageSize < 1 {
		return false, fmt.Errorf(
			"%w: page and pageSize must both be omitted or be positive integers",
			ErrInvalidPagination,
		)
	}
	if pageSize > MaxPageSize {
		return false, fmt.Errorf(
			"%w: pageSize must not exceed %d",
			ErrInvalidPagination,
			MaxPageSize,
		)
	}

	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/pageSize {
		return false, fmt.Errorf("%w: page offset is too large", ErrInvalidPagination)
	}

	return true, nil
}
