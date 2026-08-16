package panel

import "testing"

// The grammar the shell is served for, checked at the function rather than
// through a request.
//
// TestPanelServesRewrittenAssetsAndSPAFallback covers what a browser asks for.
// Two cases cannot be reached that way: an address with an empty segment is
// refused by fs.ValidPath before this function is consulted, and the shape of
// the refusal is worth pinning anyway, because this function has to be right on
// its own rather than because of what the caller happens to check first.
func TestPanelNavigationGrammar(t *testing.T) {
	served := []string{
		"inbox",
		"root",
		"root/installations",
		"root/access",
		"root/access/users/octocat/ban",
		"root/access/invitations/new",
		"root/installations/acme/history/audit",
		"root/installations/acme/repositories/api-gateway/file",
		"i/acme/settings",
		"i/acme/history",
		"i/acme/history/failures",
		"i/acme/repositories/api-gateway",
		"i/acme/users/add",
		"i/acme/invitations/inv-1/revoke",
	}
	refused := []string{
		"",
		"inbox/security",
		"root/installations/acme",
		"root/installations//repositories",
		"root/access/owners",
		"root/access/users/octocat/ban/extra",
		"i/acme/settings/anything",
		"i/acme/history/everything",
		"i//repositories",
		"i/acme/repositories//file",
		"i/acme/repositories/api-gateway/file/extra",
		"i/acme/inbox",
	}
	for _, path := range served {
		if !isPanelNavigationPath(path) {
			t.Errorf("panel navigation path %q was refused", path)
		}
	}
	for _, path := range refused {
		if isPanelNavigationPath(path) {
			t.Errorf("panel navigation path %q was served", path)
		}
	}
}
