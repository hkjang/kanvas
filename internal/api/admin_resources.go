package api

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	value, err := s.Store.AdminOverview(r.Context())
	respond(w, value, err)
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	users, err := s.Store.AdminUsers(r.Context(), r.URL.Query().Get("q"))
	respond(w, users, err)
}

func (s *Server) updateAdminUser(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	userID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}
	var input struct {
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	value, err := s.Store.UpdateAdminUser(r.Context(), identity.User.ID, userID, input.Role, input.Status)
	if err == nil {
		s.Store.Audit(r.Context(), &identity.User.ID, "ADMIN_USER_UPDATE", "USER", userID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"role": value.Role, "status": value.Status})
	}
	respond(w, value, err)
}

func (s *Server) adminGroups(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		groups, err := s.Store.AdminGroups(r.Context())
		respond(w, groups, err)
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(strings.TrimSpace(input.Name)) > 120 {
		writeError(w, http.StatusBadRequest, "group name must be 120 characters or fewer")
		return
	}
	value, err := s.Store.CreateAdminGroup(r.Context(), input.Name, input.Description)
	if err == nil {
		s.Store.Audit(r.Context(), &identity.User.ID, "ADMIN_GROUP_CREATE", "GROUP", value.ID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"name": value.Name})
	}
	respondStatus(w, http.StatusCreated, value, err)
}

func (s *Server) adminGroupMembers(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	groupID, err := uuid.Parse(r.PathValue("groupID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group ID")
		return
	}
	if r.Method == http.MethodGet {
		members, err := s.Store.AdminGroupMembers(r.Context(), groupID)
		respond(w, members, err)
		return
	}
	var input struct {
		UserID uuid.UUID `json:"userId"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.UserID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	err = s.Store.AddAdminGroupMember(r.Context(), groupID, input.UserID)
	if err == nil {
		s.Store.Audit(r.Context(), &identity.User.ID, "ADMIN_GROUP_MEMBER_ADD", "GROUP", groupID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"userId": input.UserID})
	}
	respond(w, map[string]any{"added": err == nil}, err)
}

func (s *Server) removeAdminGroupMember(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	groupID, err := uuid.Parse(r.PathValue("groupID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group ID")
		return
	}
	userID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user ID")
		return
	}
	err = s.Store.RemoveAdminGroupMember(r.Context(), groupID, userID)
	if err == nil {
		s.Store.Audit(r.Context(), &identity.User.ID, "ADMIN_GROUP_MEMBER_REMOVE", "GROUP", groupID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"userId": userID})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	respond(w, nil, err)
}

func (s *Server) adminSpaces(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	spaces, err := s.Store.AdminSpaces(r.Context())
	respond(w, spaces, err)
}

func (s *Server) updateAdminSpace(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	spaceID, err := uuid.Parse(r.PathValue("spaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid space ID")
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	value, err := s.Store.UpdateAdminSpaceStatus(r.Context(), spaceID, input.Status)
	if err == nil {
		s.Store.Audit(r.Context(), &identity.User.ID, "ADMIN_SPACE_STATUS_UPDATE", "SPACE", spaceID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"status": value.Status})
	}
	respond(w, value, err)
}
