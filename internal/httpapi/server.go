package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-canalclear-g01/internal/auth"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/clearance"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/domain"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/passage"
	"github.com/VanceMichael/go-base-canalclear-g01/internal/store"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	db         *store.Database
	sessionTTL time.Duration
	now        func() time.Time
}

type identityKey struct{}

type identity struct {
	user  auth.User
	token string
}

func New(db *store.Database, sessionTTL time.Duration) *Server {
	return &Server{db: db, sessionTTL: sessionTTL, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Server) Handler() http.Handler {
	router := chi.NewRouter()
	router.Use(requestContext)
	router.Use(securityHeaders)
	router.Use(func(next http.Handler) http.Handler { return recoverPanic(slog.Default(), next) })
	router.Use(func(next http.Handler) http.Handler { return accessLog(slog.Default(), next) })
	router.Get("/healthz", s.health)
	router.Get("/readyz", s.ready)
	router.Post("/v1/auth/login", s.login)
	router.Group(func(protected chi.Router) {
		protected.Use(s.authenticate)
		protected.Post("/v1/auth/logout", s.logout)
		protected.Post("/v1/voyages", s.declareVoyage)
		protected.Get("/v1/voyages", s.listVoyages)
		protected.Get("/v1/voyages/{id}", s.getVoyage)
		protected.Get("/v1/audit/events", s.listAudit)
		protected.Post("/v1/voyages/{id}/inspections", s.openInspection)
		protected.Post("/v1/inspections/{id}/pass", s.passInspection)
		protected.Post("/v1/voyages/{id}/passage-reservations", s.reservePassage)
	})
	return router
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ready(ctx); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ready": true})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var request struct{ Email, Password string }
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, err)
		return
	}
	token, session, err := s.db.Login(r.Context(), request.Email, request.Password, s.now(), s.sessionTTL)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": session.ExpiresAt, "role": session.Role})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	if err := s.db.Logout(r.Context(), current.token, s.now()); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) declareVoyage(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	var request struct {
		ID              string                `json:"id"`
		VesselIMO       string                `json:"vessel_imo"`
		VesselName      string                `json:"vessel_name"`
		OriginPort      string                `json:"origin_port"`
		DestinationPort string                `json:"destination_port"`
		ETA             time.Time             `json:"eta"`
		Items           []clearance.CargoItem `json:"items"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, err)
		return
	}
	voyage, err := s.db.DeclareVoyage(r.Context(), current.user, store.DeclareRequest{ID: request.ID, VesselIMO: request.VesselIMO, VesselName: request.VesselName, OriginPort: request.OriginPort, DestinationPort: request.DestinationPort, ETA: request.ETA, Items: request.Items, At: s.now()})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, voyage)
}

func (s *Server) getVoyage(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	voyage, err := s.db.GetVoyage(r.Context(), current.user.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, voyage)
}

func (s *Server) openInspection(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	var request struct{ ID, Kind string }
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, err)
		return
	}
	inspection, voyage, err := s.db.OpenInspection(r.Context(), current.user, request.ID, chi.URLParam(r, "id"), request.Kind, s.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"inspection": inspection, "voyage": voyage})
}

func (s *Server) passInspection(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	var request struct{ Finding string }
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, err)
		return
	}
	voyage, err := s.db.PassInspectionAndRelease(r.Context(), current.user, chi.URLParam(r, "id"), request.Finding, s.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, voyage)
}

func (s *Server) reservePassage(w http.ResponseWriter, r *http.Request) {
	current := r.Context().Value(identityKey{}).(identity)
	var request struct {
		ID        string
		ChamberID string    `json:"chamber_id"`
		StartsAt  time.Time `json:"starts_at"`
		EndsAt    time.Time `json:"ends_at"`
		LengthM   float64   `json:"length_m"`
		BeamM     float64   `json:"beam_m"`
		DraftM    float64   `json:"draft_m"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, err)
		return
	}
	chamber := passage.Chamber{ID: request.ChamberID, Name: request.ChamberID, MaxLengthM: 220, MaxBeamM: 34, MaxDraftM: 8}
	reservation, err := s.db.ReservePassage(r.Context(), current.user, request.ID, chi.URLParam(r, "id"), chamber, request.StartsAt, request.EndsAt, request.LengthM, request.BeamM, request.DraftM, s.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, reservation)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, r, domain.ErrForbidden)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		user, _, err := s.db.Authenticate(r.Context(), token, s.now())
		if err != nil {
			writeError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), identityKey{}, identity{user: user, token: token})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return domain.ErrInvalid
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
