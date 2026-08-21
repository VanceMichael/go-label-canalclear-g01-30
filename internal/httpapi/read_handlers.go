package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/audit"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
)

func (s *Server) listVoyages(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	if err := auth.Authorize(current.user, current.user.TenantID, auth.PermissionVoyageRead); err != nil {
		writeError(w, r, err)
		return
	}
	filter := clearance.VoyageFilter{Port: r.URL.Query().Get("port"), VesselText: r.URL.Query().Get("vessel")}
	for _, status := range r.URL.Query()["status"] {
		for _, value := range strings.Split(status, ",") {
			if value = strings.TrimSpace(value); value != "" {
				filter.Statuses = append(filter.Statuses, clearance.VoyageStatus(value))
			}
		}
	}
	if value := r.URL.Query().Get("eta_from"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			writeError(w, r, domain.ErrInvalid)
			return
		}
		filter.ETAFrom = &parsed
	}
	if value := r.URL.Query().Get("eta_to"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			writeError(w, r, domain.ErrInvalid)
			return
		}
		filter.ETATo = &parsed
	}
	page, err := s.db.ListVoyages(r.Context(), current.user.TenantID, filter, pageRequest(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	if err := auth.Authorize(current.user, current.user.TenantID, auth.PermissionAuditRead); err != nil {
		writeError(w, r, err)
		return
	}
	filter := audit.Filter{ActorID: r.URL.Query().Get("actor_id"), Action: r.URL.Query().Get("action"), ObjectID: r.URL.Query().Get("object_id"), Outcome: r.URL.Query().Get("outcome")}
	page, err := s.db.ListAuditEvents(r.Context(), current.user.TenantID, filter, pageRequest(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	page.Items = audit.SanitizeEvents(page.Items, audit.DefaultDetailSanitizer())
	writeJSON(w, http.StatusOK, page)
}

func pageRequest(r *http.Request) domain.PageRequest {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return domain.PageRequest{Limit: limit, Cursor: r.URL.Query().Get("cursor")}
}
