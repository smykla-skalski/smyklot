package orgsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strconv"

	"github.com/smykla-skalski/smyklot/pkg/config"
)

// DigestConfig fingerprints one kind's configuration.
//
// What it covers is what would change the work: whether the kind is on, and
// what it says. Who saved it and when are deliberately outside, because a
// configuration re-saved unchanged must not invalidate a plan somebody is
// halfway through reading.
func DigestConfig(enabled bool, document []byte) string {
	sum := sha256.New()

	writeField(sum, strconv.FormatBool(enabled))
	writeField(sum, string(document))

	return hex.EncodeToString(sum.Sum(nil))
}

// DigestRepositoryKind fingerprints what decides one repository's work for one
// kind: the kind's own configuration, and that repository's answer about it.
//
// This is what sync_repository_state stores and what the planner compares
// against, which is why a steady-state reconcile costs nothing: where the two
// match, the repository already has what the configuration asks for and there
// is no reason to ask GitHub what it looks like.
//
// The planner and the executor both call this, so the value recorded is the
// value the next plan will test - two spellings of the same idea would drift
// and the drift would look like a repository that never settles.
func DigestRepositoryKind(configDigest string, override *RepositoryOverride) string {
	return DigestRepositoryKindWithInputs(configDigest, override, nil)
}

// DigestInput names one additional typed decision included in a scope.
type DigestInput struct {
	Name   string
	Digest string
}

// DigestRepositoryKindWithInputs includes decisions used only by selected
// kinds, such as the effective formatting policy for synchronized files.
func DigestRepositoryKindWithInputs(
	configDigest string,
	override *RepositoryOverride,
	inputs []DigestInput,
) string {
	sum := sha256.New()

	writeField(sum, configDigest)
	writeField(sum, describeOverride(override))
	for _, input := range inputs {
		writeField(sum, input.Name)
		writeField(sum, input.Digest)
	}

	return hex.EncodeToString(sum.Sum(nil))
}

// DigestScope fingerprints everything a plan for one installation was computed
// from: each kind's configuration, and every repository override that decides
// whether a repository is in scope at all.
//
// Both halves, because either one changing changes the answer. Turning a kind
// off for one repository removes its actions just as surely as removing a
// label does, and a plan computed before that must not still be approvable.
func DigestScope(configs []Config, overrides []RepositoryOverride) string {
	return DigestScopeWithInputs(configs, overrides, nil)
}

// DigestScopeWithInputs fingerprints the ordinary sync scope plus named
// decisions that can alter planned output without changing sync documents.
func DigestScopeWithInputs(
	configs []Config,
	overrides []RepositoryOverride,
	inputs []DigestInput,
) string {
	entries := make([]string, 0, len(configs)+len(overrides)+len(inputs))

	for _, config := range configs {
		entries = append(entries, "config\x00"+string(config.Kind)+"\x00"+config.Digest)
	}

	for _, override := range overrides {
		entries = append(entries,
			"override\x00"+override.RepositoryID+"\x00"+string(override.Kind)+
				"\x00"+describeOverride(&override))
	}
	for _, input := range inputs {
		entries = append(entries, "input\x00"+input.Name+"\x00"+input.Digest)
	}

	// Sorted, because neither list arrives in a guaranteed order and a digest
	// that depended on one would mark every plan stale the first time a query
	// planner changed its mind.
	sort.Strings(entries)

	sum := sha256.New()
	for _, entry := range entries {
		writeField(sum, entry)
	}

	return hex.EncodeToString(sum.Sum(nil))
}

// DigestFormattingPolicy fingerprints a complete formatting decision without
// exposing its representation to planner callers.
func DigestFormattingPolicy(policy config.FormattingPolicy) string {
	encoded, err := json.Marshal(policy)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:])
}

// describeOverride renders a repository's whole answer about one kind: whether
// it runs, and what the repository adjusts about it.
//
// Both halves, because either one changing changes the work. A repository that
// edits its own adjustments and nothing else has to be planned again, and
// leaving them out is how it would keep the file it had while the configuration
// said something new.
//
// A nil override is one the repository has never given, which is not the same
// as one that says no: rendering it as "false" would make "inherits, and the
// installation says no" identical to "this repository says no", and those stop
// being the same the moment the installation changes its mind.
func describeOverride(override *RepositoryOverride) string {
	if override == nil {
		return "inherit"
	}

	enabled := "inherit"
	if override.Enabled != nil {
		enabled = strconv.FormatBool(*override.Enabled)
	}

	if override.AdjustsNothing() {
		return enabled
	}

	return enabled + "\x00" + string(override.Document)
}

// writeField length-prefixes a value so that a sequence of them can only be
// read one way.
//
// Concatenating instead would let two different sequences hash the same,
// because nothing marks where one value ends: ["ab", "c"] and ["a", "bc"] are
// one string. Nothing that reaches DigestScope today can construct that - the
// kinds are a closed set and a digest is hexadecimal - but the framing costs
// two bytes and its absence is the one kind of fault a fingerprint cannot
// report about itself.
func writeField(into io.Writer, value string) {
	_, _ = into.Write([]byte(strconv.Itoa(len(value))))
	_, _ = into.Write([]byte{0})
	_, _ = into.Write([]byte(value))
}
