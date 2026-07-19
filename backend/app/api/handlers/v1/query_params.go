package v1

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func queryUUIDList(params url.Values, key string) []uuid.UUID {
	return lo.FilterMap(params[key], func(id string, _ int) (uuid.UUID, bool) {
		uid, err := uuid.Parse(id)
		return uid, err == nil
	})
}

func queryIntOrNegativeOne(s string) int {
	if s == "" {
		return -1
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

func paginationRequestError(err error) error {
	status := http.StatusInternalServerError
	if errors.Is(err, repo.ErrInvalidPagination) {
		status = http.StatusBadRequest
	}
	return validate.NewRequestError(err, status)
}

func queryBool(s string) bool {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false
	}
	return b
}
