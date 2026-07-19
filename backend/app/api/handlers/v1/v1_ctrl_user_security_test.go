package v1

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

func assertRequestStatus(t *testing.T, err error, want int) {
	t.Helper()
	var requestErr *validate.RequestError
	require.ErrorAs(t, err, &requestErr)
	assert.Equal(t, want, requestErr.Status)
	assert.NotPanics(t, func() { _ = requestErr.Error() })
}

func TestHandleUserSelfChangePasswordRejectsMalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/v1/users/change-password", strings.NewReader("{"))
	err := (&V1Controller{}).HandleUserSelfChangePassword()(httptest.NewRecorder(), req)
	assertRequestStatus(t, err, http.StatusBadRequest)
}

func TestChangePasswordExpectedErrorsAreClientErrors(t *testing.T) {
	for _, err := range []error{
		services.ErrorCurrentPasswordWrong,
		services.ErrorPasswordTooShort,
	} {
		assertRequestStatus(t, changePasswordRequestError(err), http.StatusBadRequest)
	}
	assertRequestStatus(t, changePasswordRequestError(errors.New("database failed")), http.StatusInternalServerError)
}

func TestHandleUserRegistrationRejectsMalformedJSON(t *testing.T) {
	ctrl := &V1Controller{
		config: &config.Config{Options: config.Options{AllowLocalLogin: true}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/users/register", strings.NewReader("{"))
	err := ctrl.HandleUserRegistration()(httptest.NewRecorder(), req)
	assertRequestStatus(t, err, http.StatusBadRequest)
}

func TestRegistrationExpectedErrorsHaveStableStatuses(t *testing.T) {
	assertRequestStatus(t, registrationRequestError(services.ErrorPasswordTooShort), http.StatusBadRequest)
	assertRequestStatus(t, registrationRequestError(repo.ErrInvitationExpired), http.StatusBadRequest)
	assertRequestStatus(t, registrationRequestError(repo.ErrInvitationExhausted), http.StatusBadRequest)
	assertRequestStatus(t, registrationRequestError(services.ErrorEmailAlreadyExists), http.StatusConflict)
	assertRequestStatus(t, registrationRequestError(errors.New("database failed")), http.StatusInternalServerError)
}
