package httpserver

import (
	"errors"
	"net/http"

	"github.com/SkyC0der/skyroom/internal/community"
	"github.com/SkyC0der/skyroom/internal/identity"
)

type communityHandlers struct {
	service *community.Service
}

type createCommunityRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type createInvitationRequest struct {
	MaxUses *int `json:"max_uses"`
}

func (h communityHandlers) create(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	var request createCommunityRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	created, err := h.service.Create(r.Context(), token, request.Name, request.Description)
	if err != nil {
		writeCommunityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h communityHandlers) list(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	communities, err := h.service.List(r.Context(), token)
	if err != nil {
		writeCommunityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"communities": communities})
}

func (h communityHandlers) listRooms(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	rooms, err := h.service.ListRooms(r.Context(), token, r.PathValue("communityID"))
	if err != nil {
		writeCommunityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

func (h communityHandlers) createInvitation(w http.ResponseWriter, r *http.Request) {
	token, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	var request createInvitationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	invitation, err := h.service.CreateInvitation(r.Context(), token, r.PathValue("communityID"), request.MaxUses)
	if err != nil {
		writeCommunityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, invitation)
}

func (h communityHandlers) previewInvitation(w http.ResponseWriter, r *http.Request) {
	preview, err := h.service.PreviewInvitation(r.Context(), r.PathValue("token"))
	if err != nil {
		writeCommunityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (h communityHandlers) joinInvitation(w http.ResponseWriter, r *http.Request) {
	sessionToken, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	joined, err := h.service.Join(r.Context(), sessionToken, r.PathValue("token"))
	if err != nil {
		writeCommunityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, joined)
}

func (h communityHandlers) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	sessionToken, ok := readSessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	if err := h.service.RevokeInvitation(r.Context(), sessionToken, r.PathValue("communityID"), r.PathValue("invitationID")); err != nil {
		writeCommunityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeCommunityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrUnauthenticated):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
	case errors.Is(err, community.ErrHandleRequired):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "handle_required"})
	case errors.Is(err, community.ErrInvalidCommunity):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_community"})
	case errors.Is(err, community.ErrOwnershipLimit):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "owned_community_limit"})
	case errors.Is(err, community.ErrNotAuthorized):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not_authorized"})
	case errors.Is(err, community.ErrInvalidInvitation):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invitation_not_found"})
	case errors.Is(err, community.ErrInvitationExpired):
		writeJSON(w, http.StatusGone, map[string]string{"error": "invitation_expired"})
	case errors.Is(err, community.ErrInvitationRevoked):
		writeJSON(w, http.StatusGone, map[string]string{"error": "invitation_revoked"})
	case errors.Is(err, community.ErrInvitationExhausted):
		writeJSON(w, http.StatusGone, map[string]string{"error": "invitation_exhausted"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "community_operation_failed"})
	}
}
