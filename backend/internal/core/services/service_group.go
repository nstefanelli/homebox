package services

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/pkgs/hasher"
)

var (
	ErrGroupOwnerRequired    = errors.New("only group owners can perform this action")
	ErrGroupOwnerCannotLeave = errors.New("group owners cannot leave their group; delete the group instead")
	ErrGroupMemberNotFound   = errors.New("user is not a member of this group")
	ErrInvalidGroupName      = errors.New("group name must be between 1 and 255 characters")
	ErrInvalidGroupCurrency  = errors.New("group currency is required")
	ErrInvalidInvitation     = errors.New("invitation uses and expiration are invalid")
)

type GroupService struct {
	repos *repo.AllRepos
}

func (svc *GroupService) requireOwner(ctx Context) error {
	isOwner, err := svc.repos.Groups.IsOwnerOf(ctx.Context, ctx.UID, ctx.GID)
	if err != nil {
		return err
	}
	if !isOwner {
		return ErrGroupOwnerRequired
	}
	return nil
}

func (svc *GroupService) UpdateGroup(ctx Context, data repo.GroupUpdate) (repo.Group, error) {
	if err := svc.requireOwner(ctx); err != nil {
		return repo.Group{}, err
	}
	data.Name = strings.TrimSpace(data.Name)
	if data.Name == "" || utf8.RuneCountInString(data.Name) > 255 {
		return repo.Group{}, ErrInvalidGroupName
	}

	data.Currency = strings.TrimSpace(data.Currency)
	if data.Currency == "" {
		return repo.Group{}, ErrInvalidGroupCurrency
	}
	return svc.repos.Groups.GroupUpdate(ctx.Context, ctx.GID, data)
}

func (svc *GroupService) CreateGroup(ctx Context, name string) (repo.Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 255 {
		return repo.Group{}, ErrInvalidGroupName
	}

	if ctx.UID == uuid.Nil {
		return repo.Group{}, errors.New("user ID cannot be empty when creating a group")
	}

	group, err := svc.repos.Groups.GroupCreate(ctx.Context, name, ctx.UID)
	if err != nil {
		return repo.Group{}, err
	}

	// Unlike registration, this path doesn't seed default locations/tags, so
	// nothing would lazily create the entity types — leaving the collection
	// unable to create items or locations until a type was added by hand.
	if err := ensureDefaultEntityTypes(ctx.Context, svc.repos, group.ID); err != nil {
		return repo.Group{}, err
	}

	return group, nil
}

func (svc *GroupService) DeleteGroup(ctx Context) error {
	if err := svc.requireOwner(ctx); err != nil {
		return err
	}
	return svc.repos.Groups.GroupDelete(ctx.Context, ctx.GID)
}

func (svc *GroupService) NewInvitation(ctx Context, uses int, expiresAt time.Time) (repo.GroupInvitation, string, error) {
	if err := svc.requireOwner(ctx); err != nil {
		return repo.GroupInvitation{}, "", err
	}
	if uses < 1 || uses > 100 || !expiresAt.After(time.Now()) {
		return repo.GroupInvitation{}, "", ErrInvalidInvitation
	}

	token := hasher.GenerateToken()

	invitation, err := svc.repos.Groups.InvitationCreate(ctx, ctx.GID, repo.GroupInvitationCreate{
		Token:     token.Hash,
		Uses:      uses,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return repo.GroupInvitation{}, "", err
	}

	return invitation, token.Raw, nil
}

func (svc *GroupService) GetInvitations(ctx Context) ([]repo.GroupInvitation, error) {
	if err := svc.requireOwner(ctx); err != nil {
		return nil, err
	}
	return svc.repos.Groups.InvitationGetAll(ctx.Context, ctx.GID)
}

func (svc *GroupService) RemoveMember(ctx Context, userID uuid.UUID) error {
	if userID == uuid.Nil {
		return errors.New("user ID cannot be empty")
	}
	if userID != ctx.UID {
		if err := svc.requireOwner(ctx); err != nil {
			return err
		}
	}

	isMember, err := svc.repos.Groups.IsMember(ctx.Context, ctx.GID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrGroupMemberNotFound
	}

	if userID == ctx.UID {
		isOwner, err := svc.repos.Groups.IsOwnerOf(ctx.Context, ctx.UID, ctx.GID)
		if err != nil {
			return err
		}
		if isOwner {
			return ErrGroupOwnerCannotLeave
		}
	}

	return svc.repos.Groups.RemoveMemberAndReassignDefault(ctx.Context, userID, ctx.GID)
}

func (svc *GroupService) DeleteInvitation(ctx Context, id uuid.UUID) error {
	if err := svc.requireOwner(ctx); err != nil {
		return err
	}
	return svc.repos.Groups.InvitationDelete(ctx.Context, ctx.GID, id)
}

func (svc *GroupService) AcceptInvitation(ctx Context, token string) (repo.Group, error) {
	hashedToken := hasher.HashToken(token)
	return svc.repos.Groups.InvitationAccept(ctx.Context, hashedToken, ctx.UID)
}
