package panel

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

/*
Completing a GitHub login as it is typed.

The dialogs that add or invite somebody used to take a login typed in full and
say nothing until it was submitted, at which point a typo came back as "GitHub
user could not be resolved". The names being typed are almost always people in
the organization the installation belongs to, and the panel can read that roster,
so it offers it.

It is completion, not a picker. Whatever is typed is still what gets submitted,
and a login the roster does not carry - somebody outside the organization, or a
personal installation, which has no roster at all - resolves on submit exactly as
it did before.

GitHub's own user search is not one of the endpoints a GitHub App installation
token may call, and the panel holds no other credential: it reads the signed-in
person's profile once at sign-in and discards the OAuth token rather than keeping
one per session, which is what makes the consent screen ask for nothing. Storing
those tokens to power a typeahead would be a poor trade, so the roster is the
source, and it is the more useful one anyway.
*/

// maxUserSuggestions caps a response. A completion list is read at a glance, and
// a longer one is a list to search rather than an answer.
const maxUserSuggestions = 8

// minSuggestionPrefix is the shortest query that gets an answer. One letter
// matches most of any roster, which is not a suggestion.
const minSuggestionPrefix = 2

// candidateDirectory reads the roster a login can be completed against. It is
// separate from userResolver because a deployment can resolve logins without
// being able to list anybody: an installation on a personal account has no
// roster, and the panel is expected to work there with no completion at all.
type candidateDirectory interface {
	ListTargetCandidates(context.Context, string) ([]storage.Account, error)
}

func (s *Server) getTargetUserSuggestions(w http.ResponseWriter, r *http.Request) {
	/* The same standing the dialog itself needs. Naming everyone in an
	   organization is not public information, and whoever cannot add a person has
	   no reason to be handed the list of people they could have added. */
	if _, _, _, ok := s.requireTargetUserManager(w, r); !ok {
		return
	}
	s.writeUserSuggestions(w, r, r.PathValue("target"))
}

func (s *Server) getRootTargetUserSuggestions(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.requireRootInstallationManager(w, r, false)
	if !ok {
		return
	}
	s.writeUserSuggestions(w, r, manager.TargetID)
}

func (s *Server) writeUserSuggestions(w http.ResponseWriter, r *http.Request, targetID string) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < minSuggestionPrefix {
		writeJSON(w, http.StatusOK, userSuggestionsResponse{Items: []accountResponse{}})
		return
	}

	if s.candidates == nil {
		writeJSON(w, http.StatusOK, userSuggestionsResponse{Items: []accountResponse{}})
		return
	}
	candidates, err := s.candidates.ListTargetCandidates(r.Context(), targetID)
	if err != nil {
		/* Completion is an offer, never a gate. A roster the panel could not read
		   leaves the field exactly as it was before there was any completion at
		   all, which still works, so this is not an error the reader needs. */
		writeJSON(w, http.StatusOK, userSuggestionsResponse{Items: []accountResponse{}})
		return
	}

	known, err := s.store.ListTargetPanelUsers(r.Context(), targetID, s.now().UTC())
	if err != nil {
		known = nil
	}
	held := make(map[string]struct{}, len(known))
	for _, user := range known {
		held[strings.ToLower(user.User.Account.Login)] = struct{}{}
	}

	items := make([]accountResponse, 0, maxUserSuggestions)
	for _, account := range rankCandidates(candidates, query, held) {
		if len(items) == maxUserSuggestions {
			break
		}
		items = append(items, accountDTO(account))
	}
	writeJSON(w, http.StatusOK, userSuggestionsResponse{Items: items})
}

/*
rankCandidates orders a roster against what has been typed.

A login that starts with the query comes before one that merely contains it,
which is what someone typing the first letters of a name expects to see. People
who already have access to this installation are dropped: they are not candidates
for being added to it, and offering them is an invitation to a conflict.
*/
func rankCandidates(
	candidates []storage.Account,
	query string,
	held map[string]struct{},
) []storage.Account {
	needle := strings.ToLower(query)

	type scored struct {
		account storage.Account
		rank    int
	}
	matches := make([]scored, 0, len(candidates))
	for _, account := range candidates {
		login := strings.ToLower(account.Login)
		if _, taken := held[login]; taken {
			continue
		}
		name := strings.ToLower(account.DisplayName)
		switch {
		case strings.HasPrefix(login, needle):
			matches = append(matches, scored{account, 0})
		case strings.HasPrefix(name, needle):
			matches = append(matches, scored{account, 1})
		case strings.Contains(login, needle) || strings.Contains(name, needle):
			matches = append(matches, scored{account, 2})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}

		return strings.ToLower(matches[i].account.Login) < strings.ToLower(matches[j].account.Login)
	})

	ordered := make([]storage.Account, 0, len(matches))
	for _, match := range matches {
		ordered = append(ordered, match.account)
	}

	return ordered
}
