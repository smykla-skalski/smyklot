package panel

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/smykla-skalski/smyklot/internal/storage"
)

// exportPage is how much of the audit is read at a time.
//
// The export is written as it is read rather than gathered first: an audit is the
// one table here with no ceiling, and a year of it held in memory to be handed
// over in one piece is a way to lose the service to a single press.
const exportPage = 500

var auditExportHeader = []string{
	"when",
	"actor",
	"workspace",
	"subject",
	"category",
	"action",
	"summary",
	"elevation",
}

// getRootAuditExport writes the whole filtered audit as CSV.
//
// The whole of it, not the screen: an export that stopped at the page a reader
// happened to have scrolled to would be a file that says something different every
// time it is taken, and nothing about which part it holds.
func (s *Server) getRootAuditExport(w http.ResponseWriter, r *http.Request) {
	page, ok := s.rootHistoryPage(w, r, auditHistoryOrders...)
	if !ok {
		return
	}
	categories, err := parseRootAuditCategories(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_history_query", err.Error())
		return
	}
	page.Offset = 0
	page.Limit = exportPage

	// The first page is read before a single header is written: a database that
	// will not answer is a status, and after the first byte there is no status
	// left to send.
	first, err := s.store.ListRootAudit(r.Context(), storage.RootAuditPageRequest{
		HistoryPageRequest: page, Categories: categories,
	})
	if err != nil {
		s.writeInternal(w, err)
		return
	}

	stamp := s.now().UTC().Format("2006-01-02")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set(
		"Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", "smyklot-audit-"+stamp+".csv"),
	)
	w.WriteHeader(http.StatusOK)

	out := csv.NewWriter(w)
	if err := out.Write(auditExportHeader); err != nil {
		return
	}

	result := first
	for {
		for _, event := range result.Items {
			if err := out.Write(auditExportRow(event)); err != nil {
				return
			}
		}
		out.Flush()
		if err := out.Error(); err != nil {
			// The reader hung up, or the connection broke. There is nothing left to
			// say to them, so stop reading pages for a file nobody is receiving.
			return
		}
		if result.NextOffset == 0 {
			return
		}
		page.Offset = result.NextOffset
		result, err = s.store.ListRootAudit(r.Context(), storage.RootAuditPageRequest{
			HistoryPageRequest: page, Categories: categories,
		})
		if err != nil {
			return
		}
	}
}

func auditExportRow(event storage.AppAuditEvent) []string {
	return []string{
		event.CreatedAt.UTC().Format(time.RFC3339),
		accountName(&event.Actor),
		accountName(event.Target),
		accountName(event.Subject),
		string(event.Category),
		event.Action,
		event.Summary,
		strconv.FormatBool(event.ElevationID != nil),
	}
}

// accountName is the login, because an export is read by something other than a
// person as often as by one and a login is the stable name of the two.
func accountName(account *storage.Account) string {
	if account == nil {
		return ""
	}

	return account.Login
}
