package main

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/commands"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// effectiveConfig returns base with the repository's own configuration layered
// over it.
//
// A repository without .github/smyklot.yaml gets base back untouched.
func effectiveConfig(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
	base *config.Config,
) (*config.Config, error) {
	// A failure to read is transient - the network, a rate limit, a permission
	// the App just lost - so it stays retryable and says nothing
	content, err := client.GetRepoConfig(ctx, owner, repo)
	if err != nil {
		return nil, NewConfigError(ErrConfigLoad, err)
	}

	// A failure to parse is not: the file is wrong and will stay wrong until
	// someone edits it. Callers tell the repository so, rather than retrying
	cfg, err := config.LoadRepoConfig(base, content)
	if err != nil {
		return nil, NewConfigError(ErrRepoConfigInvalid, err)
	}

	return cfg, nil
}

// reportInvalidRepoConfig tells the repository its configuration file is
// broken, when the comment that triggered this asked for something.
//
// No command runs: the file is where a repository narrows what is allowed, so
// carrying on with defaults would restore commands it had turned off. Ordinary
// discussion comments pass without a word - only a comment that wanted
// something gets an answer, so a broken file does not make the bot heckle every
// conversation in the repository.
func reportInvalidRepoConfig(
	ctx context.Context,
	client *github.Client,
	rc *RuntimeConfig,
	base *config.Config,
	cause error,
) error {
	// Parsed with the configuration the process started with, since the
	// repository's own is what just failed
	parsedCmd, parseErr := commands.ParseCommand(rc.CommentBody, base)
	if parseErr == nil && !parsedCmd.IsValid {
		return cause
	}

	prNum, err := strconv.Atoi(rc.PRNumber)
	if err != nil {
		return cause
	}

	commentID, err := strconv.Atoi(rc.CommentID)
	if err != nil {
		return cause
	}

	fb := feedback.NewRepoConfigInvalid(cause.Error())
	if err := postFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError); err != nil {
		return err
	}

	return cause
}

// repoCache remembers something read per repository for a while.
//
// The Action reads one comment and exits, so it calls the loaders directly. The
// service handles every comment in every repository it serves and sweeps all of
// them on a timer, so without this it would spend a request re-reading files
// that change far less often than it looks at them.
//
// Safe for concurrent use.
type repoCache[T any] struct {
	ttl  time.Duration
	load func(context.Context, *github.Client, string, string) (T, error)

	mu       sync.Mutex
	entries  map[string]repoCacheEntry[T]
	nextLoad uint64
}

type repoCacheEntry[T any] struct {
	value      T
	fetched    time.Time
	generation uint64
}

func newRepoCache[T any](
	ttl time.Duration,
	load func(context.Context, *github.Client, string, string) (T, error),
) *repoCache[T] {
	return &repoCache[T]{
		ttl:     ttl,
		load:    load,
		entries: make(map[string]repoCacheEntry[T]),
	}
}

// Get returns the cached value for a repository, loading it on a miss.
func (c *repoCache[T]) Get(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
) (T, error) {
	return c.GetByKey(ctx, client, repoFullName(owner, repo), owner, repo)
}

// GetByKey decouples immutable cache identity from mutable GitHub lookup
// coordinates. Service configuration uses the repository ID so a rename
// cannot leave a second, stale configuration entry behind.
func (c *repoCache[T]) GetByKey(
	ctx context.Context,
	client *github.Client,
	key, owner, repo string,
) (T, error) {
	value, ok, generation := c.lookupOrBeginLoad(key)
	if ok {
		return value, nil
	}

	loaded, err := c.load(ctx, client, owner, repo)
	if err != nil {
		var zero T

		return zero, err
	}

	value = c.storeIfNewest(key, loaded, generation)

	return value, nil
}

func (c *repoCache[T]) lookupOrBeginLoad(key string) (T, bool, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if ok && time.Since(entry.fetched) < c.ttl {
		return entry.value, true, 0
	}
	c.nextLoad++
	var zero T

	return zero, false, c.nextLoad
}

// storeIfNewest makes an older load observe a newer successful result instead
// of regressing the cache after the newer caller has already acted on it.
func (c *repoCache[T]) storeIfNewest(key string, value T, generation uint64) T {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if ok && entry.generation > generation {
		return entry.value
	}
	c.entries[key] = repoCacheEntry[T]{
		value: value, fetched: time.Now(), generation: generation,
	}

	return value
}
