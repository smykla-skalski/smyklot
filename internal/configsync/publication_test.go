package configsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

func TestPrepareProposalPreservesCommentsAndRunnerWithoutMovingRefs(t *testing.T) {
	content := "# release behavior\nrunner = 'action'\nquiet_success = true # stay quiet\n"
	source, err := ReadFileSource(config.FormatTOML, []byte(content), config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	document, err := DecodeDocument(source.Snapshot.Document, config.PanelFileRepository)
	if err != nil {
		t.Fatal(err)
	}
	document.QuietSuccess = new(false)
	semantic, err := config.EncodeJSONDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	file := RemoteFile{
		Location: remoteLocation(), Head: remoteHead, Path: ".smyklot.toml", WritePath: ".smyklot.toml",
		Source: source, Content: []byte(content),
	}
	client := scriptedRemote(t,
		remoteCall{method: "GET", path: proposalRefPath(), status: 404, answer: `{}`},
		remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[]`},
		remoteCall{method: "GET", path: "/repos/acme/web/git/commits/" + remoteHead, answer: `{"tree":{"sha":"` + strings.Repeat("b", 40) + `"}}`},
		remoteCall{
			method: "POST", path: "/repos/acme/web/git/blobs", answer: `{"sha":"` + strings.Repeat("c", 40) + `"}`,
			check: func(t *testing.T, request *http.Request) {
				body := requestBody(t, request)
				text, _ := base64.StdEncoding.DecodeString(body["content"].(string))
				for _, retained := range []string{"# release behavior", "# stay quiet", "runner = 'action'", "quiet_success = false"} {
					if !strings.Contains(string(text), retained) {
						t.Errorf("publication lost %q: %s", retained, text)
					}
				}
				if !strings.HasSuffix(string(text), "\n") || strings.HasSuffix(string(text), "\n\n") {
					t.Error("publication did not retain exactly one terminal newline")
				}
			},
		},
		remoteCall{
			method: "POST", path: "/repos/acme/web/git/trees", answer: `{"sha":"` + strings.Repeat("d", 40) + `"}`,
			check: func(t *testing.T, request *http.Request) {
				body := requestBody(t, request)
				if body["base_tree"] != strings.Repeat("b", 40) || len(body["tree"].([]any)) != 1 {
					t.Error("publication changed paths outside its configuration file")
				}
			},
		},
		remoteCall{
			method: "POST", path: "/repos/acme/web/git/commits", answer: `{"sha":"` + strings.Repeat("e", 40) + `"}`,
			check: func(t *testing.T, request *http.Request) {
				parents := requestBody(t, request)["parents"].([]any)
				if len(parents) != 1 || parents[0] != remoteHead {
					t.Errorf("publication parents = %v", parents)
				}
			},
		},
	)
	proposal, err := PrepareProposal(context.Background(), client, file, semantic, nil)
	if err != nil || proposal.Commit != strings.Repeat("e", 40) || proposal.PreviousHead != "" || proposal.Digest == "" {
		t.Fatalf("proposal = %+v (%v)", proposal, err)
	}
}

func remoteLocation() RemoteLocation {
	return RemoteLocation{Owner: "acme", Repository: "web", DefaultBranch: "main", Scope: config.PanelFileRepository}
}

func proposalRefPath() string {
	return "/repos/acme/web/git/ref/heads/smyklot/repository-configuration"
}

func requestBody(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Error(err)
	}
	return body
}

func readyProposal() Proposal {
	return Proposal{Branch: proposalBranch(config.PanelFileRepository), Commit: strings.Repeat("e", 40), DefaultHead: remoteHead, Path: ".smyklot.toml"}
}

func TestPublicationCreatesOnePRAndRecoversUncertainResponses(t *testing.T) {
	proposal := readyProposal()
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "first publication", true: "retry after response loss"}[published], func(t *testing.T) {
			calls := []remoteCall{{method: "GET", path: "/repos/acme/web/git/ref/heads/main", answer: `{"object":{"sha":"` + remoteHead + `"}}`}}
			pull := `{"number":42,"state":"open","html_url":"https://github.com/acme/web/pull/42"}`
			if published {
				calls = append(calls,
					remoteCall{method: "GET", path: proposalRefPath(), answer: `{"object":{"sha":"` + proposal.Commit + `"}}`},
					remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[` + pull + `]`})
			} else {
				calls = append(calls,
					remoteCall{method: "GET", path: proposalRefPath(), status: 404, answer: `{}`},
					remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[]`},
					remoteCall{method: "POST", path: "/repos/acme/web/git/refs", answer: `{}`},
					remoteCall{method: "POST", path: "/repos/acme/web/pulls", answer: pull})
			}
			next, err := PublishProposal(context.Background(), scriptedRemote(t, calls...), remoteLocation(), proposal)
			if err != nil || next.Number != 42 || next.URL != "https://github.com/acme/web/pull/42" {
				t.Fatalf("publication = %+v (%v)", next, err)
			}
		})
	}
}

func TestPublicationStopsBeforeWritingChangedOrClosedBranches(t *testing.T) {
	for _, problem := range []string{"source_changed", "proposal_edited", "proposal_closed"} {
		t.Run(problem, func(t *testing.T) {
			calls := []remoteCall{{method: "GET", path: "/repos/acme/web/git/ref/heads/main", answer: `{"object":{"sha":"` + remoteHead + `"}}`}}
			switch problem {
			case "source_changed":
				calls[0].answer = `{"object":{"sha":"` + strings.Repeat("f", 40) + `"}}`
			case "proposal_edited":
				calls = append(calls, remoteCall{method: "GET", path: proposalRefPath(), answer: `{"object":{"sha":"` + strings.Repeat("f", 40) + `"}}`})
			case "proposal_closed":
				calls = append(calls,
					remoteCall{method: "GET", path: proposalRefPath(), status: 404, answer: `{}`},
					remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[{"state":"closed","number":42}]`})
			}
			_, err := PublishProposal(context.Background(), scriptedRemote(t, calls...), remoteLocation(), readyProposal())
			var blocked *BlockedError
			if !errors.As(err, &blocked) || blocked.Code != problem {
				t.Fatalf("failure = %v, want %s", err, problem)
			}
		})
	}
}

func TestPublicationAdvancesOnlyKnownBranchWithoutForce(t *testing.T) {
	proposal := readyProposal()
	proposal.PreviousHead = strings.Repeat("c", 40)
	client := scriptedRemote(t,
		remoteCall{method: "GET", path: "/repos/acme/web/git/ref/heads/main", answer: `{"object":{"sha":"` + remoteHead + `"}}`},
		remoteCall{method: "GET", path: proposalRefPath(), answer: `{"object":{"sha":"` + proposal.PreviousHead + `"}}`},
		remoteCall{method: "GET", path: "/repos/acme/web/pulls", answer: `[{"number":42,"state":"open"}]`},
		remoteCall{
			method: "PATCH", path: "/repos/acme/web/git/refs/heads/smyklot/repository-configuration", answer: `{}`,
			check: func(t *testing.T, request *http.Request) {
				body := requestBody(t, request)
				if body["force"] != false || body["sha"] != proposal.Commit {
					t.Errorf("unsafe branch update: %v", body)
				}
			},
		},
	)
	if _, err := PublishProposal(context.Background(), client, remoteLocation(), proposal); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationCannotWriteTheDefaultBranch(t *testing.T) {
	location := remoteLocation()
	location.DefaultBranch = proposalBranch(location.Scope)
	client := scriptedRemote(t)
	_, err := PublishProposal(context.Background(), client, location, readyProposal())
	var blocked *BlockedError
	if !errors.As(err, &blocked) || blocked.Code != "reserved_branch" {
		t.Fatalf("publication accepted a proposal as the default branch: %v", err)
	}
	if _, err := ReadRemoteFile(context.Background(), client, location); err == nil {
		t.Fatal("reader accepted a connection whose proposal is the default branch")
	}
}
