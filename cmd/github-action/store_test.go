package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/spf13/cobra"

	"github.com/smykla-skalski/smyklot/internal/storage/open"
	"github.com/smykla-skalski/smyklot/internal/storage/storagetest"
	"github.com/smykla-skalski/smyklot/internal/storage/transfer"
)

var _ = Describe("store migrate [Unit]", func() {
	var (
		ctx       context.Context
		directory string
		from, to  string
	)

	// seededAt is fixed so what the fixture writes, and therefore what the
	// report counts, is the same on every run.
	seededAt := time.Date(2026, time.March, 14, 9, 30, 0, 0, time.UTC)

	BeforeEach(func() {
		ctx = GinkgoT().Context()
		directory = GinkgoT().TempDir()
		from = filepath.Join(directory, "from.db")
		to = filepath.Join(directory, "to.db")
	})

	// seed fills the source and closes it, the way an operator stops the
	// service before copying its database.
	seed := func() {
		GinkgoHelper()

		store, err := open.Store(ctx, from)
		Expect(err).NotTo(HaveOccurred())
		Expect(storagetest.Seed(ctx, store, seededAt)).To(Succeed())
		Expect(store.Close()).To(Succeed())
	}

	// migrate runs the command as the CLI would, and returns what it printed.
	migrate := func(args ...string) (string, error) {
		GinkgoHelper()

		cmd := &cobra.Command{RunE: runStoreMigrate}
		cmd.Flags().String(flagStoreFrom, "", descStoreFrom)
		cmd.Flags().String(flagStoreTo, "", descStoreTo)
		cmd.Flags().Bool(flagStoreForce, false, descStoreForce)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		err := cmd.ExecuteContext(ctx)

		return out.String(), err
	}

	It("copies a database and reports what it carried", func() {
		seed()

		out, err := migrate("--from", from, "--to", to)
		Expect(err).NotTo(HaveOccurred())

		for _, table := range storagetest.SeededTables() {
			Expect(out).To(MatchRegexp(`(?m)^\s+`+table+`\s+[1-9]`),
				"table %s should have carried rows", table)
		}
		Expect(out).To(MatchRegexp(`Copied \d+ rows in `))

		// The destination is a working database, not just a file that exists.
		copied, err := open.Store(ctx, to)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(copied.Close()).To(Succeed()) }()
		users, err := copied.ListPanelUsers(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(users).NotTo(BeEmpty())
	})

	It("refuses a destination that already holds rows, and says how to proceed", func() {
		seed()

		_, err := migrate("--from", from, "--to", to)
		Expect(err).NotTo(HaveOccurred())

		_, err = migrate("--from", from, "--to", to)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, ErrStoreMigrate)).To(BeTrue())
		Expect(errors.Is(err, transfer.ErrDestinationNotEmpty)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("--" + flagStoreForce))
	})

	It("empties the destination when forced, rather than doubling it", func() {
		seed()

		first, err := migrate("--from", from, "--to", to)
		Expect(err).NotTo(HaveOccurred())

		forced, err := migrate("--from", from, "--to", to, "--force")
		Expect(err).NotTo(HaveOccurred())
		Expect(rowCounts(forced)).To(Equal(rowCounts(first)))
	})

	DescribeTable("rejects an unusable request",
		func(args []string, expected string) {
			_, err := migrate(args...)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrStoreMigrate)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring(expected))
		},
		Entry("with no source", []string{"--to", "/tmp/x.db"}, "are both required"),
		Entry("with no destination", []string{"--from", "/tmp/x.db"}, "are both required"),
		Entry("copying into itself",
			[]string{"--from", "/tmp/x.db", "--to", "/tmp/x.db"}, "same database"),
		Entry("with an unknown engine",
			[]string{"--from", "mysql://host/db", "--to", "/tmp/x.db"}, "unsupported storage scheme"),
	)
})

// rowCounts drops the timing line from a report, which differs between two runs
// of the same copy and is the only thing that should.
func rowCounts(report string) string {
	counts, _, _ := strings.Cut(report, "\nCopied")

	return counts
}
