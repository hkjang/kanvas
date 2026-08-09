package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/kanvas/internal/migration"
)

func (s *Server) startReconciliation(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	job, err := s.Migration.StartReconciliation(r.Context(), identity.User.ID)
	respondStatus(w, http.StatusAccepted, job, err)
}

func (s *Server) unsupportedContent(w http.ResponseWriter, r *http.Request) {
	_, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	filter := migration.UnsupportedFilter{
		Status: r.URL.Query().Get("status"),
		Kind:   r.URL.Query().Get("kind"),
		Query:  r.URL.Query().Get("q"),
	}
	filter.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	filter.Offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if value := strings.TrimSpace(r.URL.Query().Get("jobId")); value != "" {
		jobID, err := uuid.Parse(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid snapshot job ID")
			return
		}
		filter.JobID = &jobID
	}
	page, err := s.Migration.UnsupportedContent(r.Context(), filter)
	respond(w, page, err)
}

func (s *Server) decideUnsupportedContent(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	itemID, err := uuid.Parse(r.PathValue("itemID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid unsupported content item ID")
		return
	}
	var input struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	updated, err := s.Migration.DecideUnsupportedContent(r.Context(), []uuid.UUID{itemID}, input.Status, input.Resolution, identity.User.ID)
	if err == nil {
		s.Store.Audit(r.Context(), &identity.User.ID, "UNSUPPORTED_CONTENT_DECISION", "UNSUPPORTED_CONTENT", itemID.String(), r.RemoteAddr, r.UserAgent(), map[string]any{"status": strings.ToUpper(input.Status), "resolution": input.Resolution})
	}
	respond(w, map[string]any{"updated": updated}, err)
}

func (s *Server) bulkDecideUnsupportedContent(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authorize(w, r, "ADMIN", "*")
	if !ok {
		return
	}
	var input struct {
		IDs        []uuid.UUID `json:"ids"`
		Status     string      `json:"status"`
		Resolution string      `json:"resolution"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	updated, err := s.Migration.DecideUnsupportedContent(r.Context(), input.IDs, input.Status, input.Resolution, identity.User.ID)
	if err == nil {
		s.Store.Audit(r.Context(), &identity.User.ID, "UNSUPPORTED_CONTENT_BULK_DECISION", "UNSUPPORTED_CONTENT", "bulk", r.RemoteAddr, r.UserAgent(), map[string]any{"status": strings.ToUpper(input.Status), "resolution": input.Resolution, "count": updated})
	}
	respond(w, map[string]any{"updated": updated}, err)
}
