package v1

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/internal/web/adapters"
)

type (
	GroupInvitationCreate struct {
		Uses      int       `json:"uses"      validate:"required,min=1,max=100"`
		ExpiresAt time.Time `json:"expiresAt"`
	}

	GroupInvitation struct {
		ID        uuid.UUID `json:"id"`
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expiresAt"`
		Uses      int       `json:"uses"`
	}

	GroupAcceptInvitationResponse struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	CreateRequest struct {
		Name string `json:"name" validate:"required,min=1,max=255"`
	}
)

func mapGroupActionError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, services.ErrGroupOwnerRequired):
		return validate.NewRequestError(err, http.StatusForbidden)
	case errors.Is(err, services.ErrGroupOwnerCannotLeave):
		return validate.NewRequestError(err, http.StatusConflict)
	case errors.Is(err, services.ErrGroupMemberNotFound):
		return validate.NewRequestError(err, http.StatusNotFound)
	case errors.Is(err, services.ErrInvalidGroupName),
		errors.Is(err, services.ErrInvalidGroupCurrency),
		errors.Is(err, services.ErrInvalidInvitation):
		return validate.NewRequestError(err, http.StatusBadRequest)
	default:
		return err
	}
}

func mapInvitationAcceptError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, repo.ErrAlreadyGroupMember):
		return validate.NewRequestError(err, http.StatusConflict)
	case errors.Is(err, repo.ErrInvitationExpired), errors.Is(err, repo.ErrInvitationExhausted):
		return validate.NewRequestError(err, http.StatusBadRequest)
	default:
		return err
	}
}

// HandleGroupGet godoc
//
//	@Summary	Get Group
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	repo.Group
//	@Router		/v1/groups [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupGet() errchain.HandlerFunc {
	fn := func(r *http.Request) (repo.Group, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.Groups.GroupByID(auth, auth.GID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupUpdate godoc
//
//	@Summary	Update Group
//	@Tags		Group
//	@Produce	json
//	@Param		payload	body		repo.GroupUpdate	true	"User Data"
//	@Success	200		{object}	repo.Group
//	@Router		/v1/groups [Put]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupUpdate() errchain.HandlerFunc {
	fn := func(r *http.Request, body repo.GroupUpdate) (repo.Group, error) {
		auth := services.NewContext(r.Context())
		body.Currency = strings.TrimSpace(body.Currency)

		ok := ctrl.svc.Currencies.IsSupported(body.Currency)
		if !ok {
			return repo.Group{}, validate.NewFieldErrors(
				validate.NewFieldError("currency", "currency '"+body.Currency+"' is not supported"),
			)
		}

		out, err := ctrl.svc.Group.UpdateGroup(auth, body)
		return out, mapGroupActionError(err)
	}

	return adapters.Action(fn, http.StatusOK)
}

// HandleGroupInvitationsCreate godoc
//
//	@Summary	Create Group Invitation
//	@ID			groupInvitationCreate
//	@Tags		Group
//	@Produce	json
//	@Param		payload	body		GroupInvitationCreate	true	"User Data"
//	@Success	200		{object}	GroupInvitation
//	@Router		/v1/groups/invitations [Post]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsCreate() errchain.HandlerFunc {
	fn := func(r *http.Request, body GroupInvitationCreate) (GroupInvitation, error) {
		if body.ExpiresAt.IsZero() {
			body.ExpiresAt = time.Now().Add(time.Hour * 24)
		}

		auth := services.NewContext(r.Context())

		invitation, token, err := ctrl.svc.Group.NewInvitation(auth, body.Uses, body.ExpiresAt)
		if err != nil {
			return GroupInvitation{}, mapGroupActionError(err)
		}

		return GroupInvitation{
			ID:        invitation.ID,
			Token:     token,
			ExpiresAt: invitation.ExpiresAt,
			Uses:      invitation.Uses,
		}, nil
	}

	return adapters.Action(fn, http.StatusCreated)
}

// HandleGroupsGetAll godoc
//
//	@Summary	Get All Groups
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	[]repo.Group
//	@Router		/v1/groups/all [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupsGetAll() errchain.HandlerFunc {
	fn := func(r *http.Request) ([]repo.Group, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.Groups.GetAllGroups(auth, auth.UID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupCreate godoc
//
//	@Summary	Create Group
//	@Tags		Group
//	@Produce	json
//	@Param		payload	body		CreateRequest	true	"Create group request"
//	@Success	201		{object}	repo.Group
//	@Router		/v1/groups [Post]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupCreate() errchain.HandlerFunc {
	fn := func(r *http.Request, body CreateRequest) (repo.Group, error) {
		auth := services.NewContext(r.Context())
		out, err := ctrl.svc.Group.CreateGroup(auth, body.Name)
		return out, mapGroupActionError(err)
	}

	return adapters.Action(fn, http.StatusCreated)
}

// HandleGroupDelete godoc
//
//	@Summary	Delete Group
//	@Tags		Group
//	@Produce	json
//	@Success	204
//	@Router		/v1/groups [Delete]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupDelete() errchain.HandlerFunc {
	fn := func(r *http.Request) (any, error) {
		auth := services.NewContext(r.Context())
		err := ctrl.svc.Group.DeleteGroup(auth)
		return nil, mapGroupActionError(err)
	}

	return adapters.Command(fn, http.StatusNoContent)
}

// HandleGroupInvitationsGetAll godoc
//
//	@Summary	Get All Group Invitations
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	[]repo.GroupInvitation
//	@Router		/v1/groups/invitations [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsGetAll() errchain.HandlerFunc {
	fn := func(r *http.Request) ([]repo.GroupInvitation, error) {
		auth := services.NewContext(r.Context())
		out, err := ctrl.svc.Group.GetInvitations(auth)
		return out, mapGroupActionError(err)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupMembersGetAll godoc
//
//	@Summary	Get All Group Members
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	[]repo.UserSummary
//	@Router		/v1/groups/members [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupMembersGetAll() errchain.HandlerFunc {
	fn := func(r *http.Request) ([]repo.UserSummary, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.Users.GetUsersByGroupID(auth, auth.GID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupMemberRemove godoc
//
//	@Summary	Remove User from Group
//	@Tags		Group
//	@Produce	json
//	@Param		user_id	path	string	true	"User ID"
//	@Success	204
//	@Router		/v1/groups/members/{user_id} [Delete]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupMemberRemove() errchain.HandlerFunc {
	fn := func(r *http.Request, userID uuid.UUID) (any, error) {
		auth := services.NewContext(r.Context())

		err := ctrl.svc.Group.RemoveMember(auth, userID)
		return nil, mapGroupActionError(err)
	}

	return adapters.CommandID("user_id", fn, http.StatusNoContent)
}

// HandleGroupInvitationsDelete godoc
//
//	@Summary	Delete Group Invitation
//	@Tags		Group
//	@Produce	json
//	@Param		id	path	string	true	"Invitation ID"
//	@Success	204
//	@Router		/v1/groups/invitations/{id} [Delete]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsDelete() errchain.HandlerFunc {
	fn := func(r *http.Request, id uuid.UUID) (any, error) {
		auth := services.NewContext(r.Context())
		err := ctrl.svc.Group.DeleteInvitation(auth, id)
		return nil, mapGroupActionError(err)
	}

	return adapters.CommandID("id", fn, http.StatusNoContent)
}

// HandleGroupInvitationsAccept godoc
//
//	@Summary	Accept Group Invitation
//	@ID			groupInvitationAccept
//	@Tags		Group
//	@Produce	json
//	@Param		id	path		string	true	"Invitation Token"
//	@Success	200	{object}	GroupAcceptInvitationResponse
//	@Router		/v1/groups/invitations/{id} [Post]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsAccept() errchain.HandlerFunc {
	fn := func(r *http.Request) (GroupAcceptInvitationResponse, error) {
		token := chi.URLParam(r, "id")
		if token == "" {
			return GroupAcceptInvitationResponse{}, validate.NewRequestError(errors.New("token is required"), http.StatusBadRequest)
		}

		auth := services.NewContext(r.Context())
		group, err := ctrl.svc.Group.AcceptInvitation(auth, token)
		if err != nil {
			return GroupAcceptInvitationResponse{}, mapInvitationAcceptError(err)
		}

		return GroupAcceptInvitationResponse{ID: group.ID, Name: group.Name}, nil
	}

	return adapters.Command(fn, http.StatusOK)
}
