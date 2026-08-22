package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/internal/storage/transfer"
)

const (
	flagStoreFrom  = "from"
	flagStoreTo    = "to"
	flagStoreForce = "force"

	descStoreFrom  = "Database to read from: a postgres:// URL or a file path"
	descStoreTo    = "Database to write to: a postgres:// URL or a file path"
	descStoreForce = "Empty the destination first, instead of refusing one that holds rows"
)

// ErrStoreMigrate is returned when service state cannot be copied.
var ErrStoreMigrate = errors.New("cannot migrate service state")

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Work with the service's database",
}

var storeMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Copy service state from one database to another",
	Long: `Copy every row of service state from one database into another.

Use this to change storage engines without starting from an empty database:
moving a SQLite file onto a PostgreSQL server, or back off one.

The destination is migrated to the current schema first, and must then be
empty. Copying into a database that already holds rows would merge two
histories and is refused; --force empties it first instead. Everything runs in
one transaction, so a copy that fails leaves the destination as it found it.

Stop the service before running this. Rows written after the copy has read a
table would not be carried, and the report would still say it succeeded.

Both databases are brought to the current schema, the source included - the
copy reads its columns, and a source a few releases behind would be missing
some. So the source is written to. Work from a copy of anything irreplaceable.

  smyklot store migrate \
      --from /var/lib/smyklot/panel.sqlite3 \
      --to 'postgres://smyklot@smyklot-db.internal:5432/smyklot'`,
	RunE: runStoreMigrate,
}

func init() {
	storeMigrateCmd.Flags().String(flagStoreFrom, "", descStoreFrom)
	storeMigrateCmd.Flags().String(flagStoreTo, "", descStoreTo)
	storeMigrateCmd.Flags().Bool(flagStoreForce, false, descStoreForce)

	storeCmd.AddCommand(storeMigrateCmd)
	rootCmd.AddCommand(storeCmd)
}

func runStoreMigrate(cmd *cobra.Command, _ []string) error {
	from, err := cmd.Flags().GetString(flagStoreFrom)
	if err != nil {
		return err
	}
	to, err := cmd.Flags().GetString(flagStoreTo)
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool(flagStoreForce)
	if err != nil {
		return err
	}

	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return fmt.Errorf("%w: --%s and --%s are both required",
			ErrStoreMigrate, flagStoreFrom, flagStoreTo)
	}

	// Past here the invocation is well formed, and anything that fails is the
	// copy rather than the request. Printing usage under those errors buries
	// the one line that says what went wrong.
	cmd.SilenceUsage = true

	report, err := transfer.Between(cmd.Context(), from, to, transfer.Options{Force: force})
	if err != nil {
		if errors.Is(err, transfer.ErrDestinationNotEmpty) {
			return fmt.Errorf("%w: %w (use --%s to empty it first)",
				ErrStoreMigrate, err, flagStoreForce)
		}

		return fmt.Errorf("%w: %w", ErrStoreMigrate, err)
	}

	cmd.Print(formatReport(report))

	return nil
}

// formatReport renders what was copied, one table per line with the counts
// lined up so a table that carried nothing is easy to spot.
func formatReport(report transfer.Report) string {
	names := make([]string, 0, len(report.Rows))
	width := 0
	for name := range report.Rows {
		names = append(names, name)
		if len(name) > width {
			width = len(name)
		}
	}
	sort.Strings(names)

	var out strings.Builder
	for _, name := range names {
		fmt.Fprintf(&out, "  %-*s  %d\n", width, name, report.Rows[name])
	}
	fmt.Fprintf(&out, "\nCopied %d rows in %s.\n", report.Total(), report.Duration.Round(time.Millisecond))

	return out.String()
}
