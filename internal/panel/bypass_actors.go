package panel

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

type bypassActorDirectory interface {
	LookupBypassActors(context.Context, string, string, string) ([]github.BypassActorIdentity, bool, error)
}

type bypassActorsResponse struct {
	Items   []github.BypassActorIdentity `json:"items"`
	Warning string                       `json:"warning,omitempty"`
}

func (s *Server) getTargetBypassActors(w http.ResponseWriter, r *http.Request) {
	_, target, _, ok := s.requireTarget(w, r, false)
	if !ok {
		return
	}
	s.writeBypassActors(w, r, target.ID)
}

func (s *Server) getRootBypassActors(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootWorkspaceManager(w, r, false)
	if !ok {
		return
	}
	s.writeBypassActors(w, r, manager.TargetID)
}

func (s *Server) writeBypassActors(w http.ResponseWriter, r *http.Request, targetID string) {
	kind, query := r.URL.Query().Get("type"), strings.TrimSpace(r.URL.Query().Get("q"))
	if (kind != "" && kind != "Integration" && kind != "Team" && kind != "User") ||
		(query != "" && (kind == "" || utf8.RuneCountInString(query) > 100 || strings.ContainsAny(query, "/\\\r\n\t"))) {
		s.writeError(w, http.StatusBadRequest, "invalid_actor_query", "Enter an app slug, team slug, or user login")
		return
	}
	if s.bypassActors == nil {
		writeJSON(w, http.StatusOK, bypassActorsResponse{Items: []github.BypassActorIdentity{}, Warning: "GitHub actor lookup is unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	items, complete, err := s.bypassActors.LookupBypassActors(ctx, targetID, kind, query)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "github_actor_unavailable", "GitHub could not resolve these actors. Check the name and try again")
		return
	}
	if items == nil {
		items = []github.BypassActorIdentity{}
	}
	response := bypassActorsResponse{Items: items}
	if !complete {
		response.Warning = "Some GitHub names or installation details are unavailable. You can still edit existing exceptions or look up an actor by name"
	}
	writeJSON(w, http.StatusOK, response)
}
