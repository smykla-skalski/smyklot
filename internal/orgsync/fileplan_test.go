package orgsync_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/smyklot/internal/orgsync"
	"github.com/smykla-skalski/smyklot/internal/orgsync/filemerge"
	appconfig "github.com/smykla-skalski/smyklot/pkg/config"
)

func planFilesPreserving(
	repositoryID string,
	config orgsync.FileConfig,
	override orgsync.FileOverride,
	defaultBranch string,
	current map[string]orgsync.CurrentFile,
) (orgsync.FilePlan, error) {
	return orgsync.PlanFiles(
		repositoryID,
		config,
		override,
		defaultBranch,
		current,
		appconfig.DefaultFormattingPolicy(),
	)
}

// held is what a repository's tree says about a file it carries.
//
// Named by the same function the planner uses. Hashing it a second way here
// would prove the two agreed rather than that either was right, and git's
// object naming is not a thing to have two opinions about.
func held(content string) orgsync.CurrentFile {
	return orgsync.CurrentFile{
		Blob: orgsync.BlobID([]byte(content)), Size: len(content),
	}
}

var _ = Describe("Planning files [Unit]", func() {
	const (
		contributing = "# Contributing\n\nSend a pull request.\n"
		trigger      = ".github/workflows/sync-trigger.yml"
	)

	config := orgsync.FileConfig{Files: []orgsync.File{
		file("CONTRIBUTING.md", contributing),
	}}

	plan := func(
		config orgsync.FileConfig,
		override orgsync.FileOverride,
		current map[string]orgsync.CurrentFile,
	) orgsync.FilePlan {
		GinkgoHelper()

		answer, err := planFilesPreserving("repo-1", config, override, "main", current)
		Expect(err).NotTo(HaveOccurred())

		return answer
	}

	It("adds a file the repository does not have", func() {
		actions := plan(config, orgsync.FileOverride{}, nil).Actions

		Expect(actions).To(HaveLen(1))
		Expect(actions[0].Operation).To(Equal(orgsync.OperationCreate))
		Expect(actions[0].Subject).To(Equal("CONTRIBUTING.md"))
		Expect(actions[0].Before).To(BeEmpty())
		Expect(actions[0].After).To(Equal("37 bytes from the template"))

		written, err := orgsync.DecodeFile(actions[0].Payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(written.Content)).To(Equal(contributing))
	})

	It("leaves a file the repository already carries alone", func() {
		Expect(plan(config, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
			"CONTRIBUTING.md": held(contributing),
		}).Actions).To(BeEmpty())
	})

	It("changes a file that says something else", func() {
		actions := plan(config, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
			"CONTRIBUTING.md": held("# Contributing\n"),
		}).Actions

		Expect(actions).To(HaveLen(1))
		Expect(actions[0].Operation).To(Equal(orgsync.OperationUpdate))
		Expect(actions[0].Before).To(Equal("15 bytes"))
	})

	It("fills the repository's own branch into the template", func() {
		answer, err := planFilesPreserving("repo-1", orgsync.FileConfig{
			Files: []orgsync.File{file("README.md", "Built from {{DEFAULT_BRANCH}}.\n")},
		}, orgsync.FileOverride{}, "trunk", nil)

		Expect(err).NotTo(HaveOccurred())

		written, err := orgsync.DecodeFile(answer.Actions[0].Payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(written.Content)).To(Equal("Built from trunk.\n"))
	})

	It("plans a formatting-only update", func() {
		policy := appconfig.DefaultFormattingPolicy()
		policy.JSON.Arrays = "expanded"
		template := `{"labels":["one","two"]}`

		answer, err := orgsync.PlanFiles(
			"repo-1",
			orgsync.FileConfig{Files: []orgsync.File{file("config.json", template)}},
			orgsync.FileOverride{},
			"main",
			map[string]orgsync.CurrentFile{"config.json": held(template)},
			policy,
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(answer.Actions).To(HaveLen(1))
		written, err := orgsync.DecodeFile(answer.Actions[0].Payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(written.Content)).To(Equal("{\"labels\":[\n    \"one\",\n    \"two\"\n  ]}"))
	})

	It("applies template formatting before the repository path overlay", func() {
		compact, preserve := "compact", "preserve"
		policy := appconfig.DefaultFormattingPolicy()
		policy.JSON.Arrays = "expanded"
		template := "{\n  \"labels\": [\n    \"one\",\n    \"two\"\n  ]\n}\n"
		fileConfig := orgsync.FileConfig{Files: []orgsync.File{{
			Path: "config.json", Content: template,
			Formatting: &appconfig.FormattingPatch{
				JSON: &appconfig.FormattingJSONPatch{Arrays: &compact},
			},
		}}}
		override := orgsync.FileOverride{Formats: []orgsync.FileFormat{{
			Path: "config.json",
			Formatting: appconfig.FormattingPatch{
				JSON: &appconfig.FormattingJSONPatch{Arrays: &preserve},
			},
		}}}

		answer, err := orgsync.PlanFiles(
			"repo-1",
			fileConfig,
			override,
			"main",
			map[string]orgsync.CurrentFile{"config.json": held(template)},
			policy,
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(answer.Actions).To(BeEmpty())
	})

	Describe("what a repository adjusts", func() {
		renovate := orgsync.FileConfig{Files: []orgsync.File{
			file("renovate.json", `{"extends": ["config:recommended"], "timezone": "UTC"}`),
		}}

		adjusted := orgsync.FileOverride{Merges: []orgsync.FileMerge{{
			Path: "renovate.json",
			Spec: filemerge.Spec{Overrides: []byte(`{"timezone": "Europe/Warsaw"}`)},
		}}}

		It("writes the composed file rather than the template", func() {
			actions := plan(renovate, adjusted, nil).Actions

			written, err := orgsync.DecodeFile(actions[0].Payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(written.Content)).To(ContainSubstring("Europe/Warsaw"))
			Expect(string(written.Content)).To(ContainSubstring("config:recommended"))
			Expect(actions[0].After).To(HaveSuffix("adjusted for this repository"))
		})

		// The bound the configuration was held to is not a bound on what comes
		// out of the merge, and what comes out is what a plan carries - once
		// per repository it would write it to.
		It("refuses one that composes more than a repository may be sent", func() {
			_, err := planFilesPreserving("repo-1", orgsync.FileConfig{
				Files: []orgsync.File{file("renovate.json", `{"a":1}`)},
			}, orgsync.FileOverride{
				Merges: []orgsync.FileMerge{{
					Path: "renovate.json",
					Spec: filemerge.Spec{
						Overrides: []byte(
							`{"big":"` + strings.Repeat("x", 1_200_000) + `"}`),
					},
				}},
			}, "main", nil)

			Expect(err).To(MatchError(orgsync.ErrInvalidConfig))
			Expect(err.Error()).To(ContainSubstring("more than this repository may be sent"))
		})

		// Fail-closed. The tool this replaces reported the same condition as a
		// warning and wrote the raw template over the repository's file, so a
		// broken adjustment destroyed exactly the customization it described.
		It("refuses rather than writing the template over the file", func() {
			_, err := planFilesPreserving("repo-1", renovate, orgsync.FileOverride{
				Merges: []orgsync.FileMerge{{
					Path: "renovate.json",
					Spec: filemerge.Spec{
						Overrides: []byte(`{"labels": ["a"]}`),
						Arrays: []filemerge.ArrayRule{
							{Path: "$.label", Strategy: filemerge.ArrayAppend},
						},
					},
				}},
			}, "main", nil)

			Expect(err).To(MatchError(filemerge.ErrInvalidSpec))
		})
	})

	Describe("paths the organization has retired", func() {
		retiring := orgsync.FileConfig{
			Files:   []orgsync.File{file("CONTRIBUTING.md", contributing)},
			Retired: []string{trigger, ".renovaterc"},
		}

		It("removes only the ones the repository still has", func() {
			actions := plan(retiring, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
				"CONTRIBUTING.md": held(contributing),
				".renovaterc":     held("{}"),
			}).Actions

			Expect(actions).To(HaveLen(1))
			Expect(actions[0].Operation).To(Equal(orgsync.OperationDelete))
			Expect(actions[0].Subject).To(Equal(".renovaterc"))
			Expect(actions[0].Before).To(Equal("2 bytes"))
			Expect(actions[0].After).To(BeEmpty())
		})

		It("removes them in one order however the tree arrived in another", func() {
			current := map[string]orgsync.CurrentFile{
				"CONTRIBUTING.md": held(contributing),
				".renovaterc":     held("{}"),
				trigger:           held("on: push\n"),
			}

			first := plan(retiring, orgsync.FileOverride{}, current).Actions
			second := plan(retiring, orgsync.FileOverride{}, current).Actions

			Expect(subjects(first)).To(Equal([]string{trigger, ".renovaterc"}))
			Expect(subjects(second)).To(Equal(subjects(first)))
		})

		// The tool this replaces checked its exclusion list when writing files
		// and not when removing them, so a repository that had asked to keep a
		// path watched it go anyway.
		It("keeps a retired path the repository asked to be left alone", func() {
			excluding := retiring
			excluding.Excludes = []string{".renovaterc"}

			Expect(plan(excluding, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
				"CONTRIBUTING.md": held(contributing),
				".renovaterc":     held("{}"),
			}).Actions).To(BeEmpty())
		})
	})

	Describe("exclusions", func() {
		It("leaves a file the installation excludes alone", func() {
			excluding := config
			excluding.Excludes = []string{"CONTRIBUTING.md"}

			Expect(plan(excluding, orgsync.FileOverride{}, nil).Actions).To(BeEmpty())
		})

		It("leaves one the repository excludes alone", func() {
			Expect(plan(config, orgsync.FileOverride{
				Excludes: []string{"*.md"},
			}, nil).Actions).To(BeEmpty())
		})
	})

	Describe("the branch the work is proposed on", func() {
		It("is the same for two runs against one configuration", func() {
			Expect(plan(config, orgsync.FileOverride{}, nil).Proposal).
				To(Equal(plan(config, orgsync.FileOverride{}, nil).Proposal))
		})

		// Named after what the files should end up saying rather than after
		// what would change, so a repository where somebody has already fixed
		// one of them by hand keeps the pull request that is open for it.
		It("does not move when part of the work is already done", func() {
			two := orgsync.FileConfig{Files: []orgsync.File{
				file("CONTRIBUTING.md", contributing),
				file("SECURITY.md", "# Security\n"),
			}}

			whole := plan(two, orgsync.FileOverride{}, nil)
			half := plan(two, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
				"SECURITY.md": held("# Security\n"),
			})

			Expect(half.Actions).To(HaveLen(1))
			Expect(half.Proposal).To(Equal(whole.Proposal))
		})

		// A configuration that has changed is a different proposal, so a pull
		// request somebody closed does not suppress the next thing they are
		// asked about.
		It("moves when the file would say something else", func() {
			changed := orgsync.FileConfig{Files: []orgsync.File{
				file("CONTRIBUTING.md", contributing+"\nBe kind.\n"),
			}}

			Expect(plan(changed, orgsync.FileOverride{}, nil).Proposal).
				NotTo(Equal(plan(config, orgsync.FileOverride{}, nil).Proposal))
		})

		It("moves when a path is retired", func() {
			retiring := config
			retiring.Retired = []string{".renovaterc"}

			current := map[string]orgsync.CurrentFile{".renovaterc": held("{}")}

			Expect(plan(retiring, orgsync.FileOverride{}, current).Proposal).
				NotTo(Equal(plan(config, orgsync.FileOverride{}, current).Proposal))
		})

		// The same property as above, from the other side: naming the branch
		// after what is left to do would rename it the moment somebody deleted
		// one of the retired paths by hand, abandoning the pull request that is
		// open for the rest.
		It("does not move when a retired path is already gone", func() {
			retiring := config
			retiring.Retired = []string{".renovaterc", ".renovaterc.json"}

			both := plan(retiring, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
				".renovaterc":      held("{}"),
				".renovaterc.json": held("{}"),
			})
			one := plan(retiring, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
				".renovaterc": held("{}"),
			})

			Expect(subjects(both.Actions)).To(ContainElement(".renovaterc.json"))
			Expect(subjects(one.Actions)).NotTo(ContainElement(".renovaterc.json"))
			Expect(one.Proposal).To(Equal(both.Proposal))
		})

		It("differs where a repository adjusts the file", func() {
			renovate := orgsync.FileConfig{Files: []orgsync.File{
				file("renovate.json", `{"timezone": "UTC"}`),
			}}

			Expect(plan(renovate, orgsync.FileOverride{Merges: []orgsync.FileMerge{{
				Path: "renovate.json",
				Spec: filemerge.Spec{Overrides: []byte(`{"timezone": "Europe/Warsaw"}`)},
			}}}, nil).Proposal).NotTo(Equal(plan(renovate, orgsync.FileOverride{}, nil).Proposal))
		})

		It("goes on every action, because the plan is what says where work lands", func() {
			answer := plan(orgsync.FileConfig{
				Files:   []orgsync.File{file("CONTRIBUTING.md", contributing)},
				Retired: []string{".renovaterc"},
			}, orgsync.FileOverride{}, map[string]orgsync.CurrentFile{
				".renovaterc": held("{}"),
			})

			Expect(answer.Actions).To(HaveLen(2))

			for _, action := range answer.Actions {
				written, err := orgsync.DecodeFile(action.Payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(written.Proposal).To(Equal(answer.Proposal))
			}
		})

		It("names a branch under Smyklot's own prefix", func() {
			Expect(plan(config, orgsync.FileOverride{}, nil).Proposal).
				To(MatchRegexp(`^smyklot/files-[0-9a-f]{12}$`))
		})
	})
})

func subjects(actions []orgsync.Action) []string {
	found := make([]string, 0, len(actions))
	for _, action := range actions {
		found = append(found, action.Subject)
	}

	return found
}
