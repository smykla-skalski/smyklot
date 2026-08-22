package main

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/smykla-skalski/smyklot/internal/bot"
	"github.com/smykla-skalski/smyklot/pkg/commands"
	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/feedback"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// effectiveConfig returns base with the repository's own configuration layered
// over it.
//
// A repository with no configuration file gets base back untouched.
func effectiveConfig(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
	base *config.Config,
) (*config.Config, error) {
	// A failure to read is transient - the network, a rate limit, a permission
	// the App just lost - so it stays retryable and says nothing.
	//
	// This costs one request per candidate path and does not cache, which is
	// what the Action is: one comment, one process, then exit. The service is
	// the one that reads this on a timer, and it goes through repoCache.
	found, err := client.FindRepoConfig(ctx, owner, repo, "")
	if err != nil {
		return nil, bot.NewConfigError(bot.ErrConfigLoad, err)
	}
	if !found.Found() {
		return base, nil
	}

	// A failure to parse is not: the file is wrong and will stay wrong until
	// someone edits it. Callers tell the repository so, rather than retrying
	cfg, err := loadRepoConfig(base, found)
	if err != nil {
		return nil, bot.NewConfigError(bot.ErrRepoConfigInvalid, err)
	}

	return cfg, nil
}

// loadRepoConfig layers a found configuration file over base, reading it in
// whichever format its name says it is written in.
func loadRepoConfig(base *config.Config, found github.RepoConfig) (*config.Config, error) {
	format, err := config.FormatOf(found.Path)
	if err != nil {
		return nil, err
	}

	return config.LoadRepoConfig(base, format, found.Content)
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
	rc *bot.RuntimeConfig,
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
	if err := bot.PostFeedback(ctx, client, rc, prNum, commentID, fb.Message, github.ReactionError); err != nil {
		return err
	}

	return cause
}

// errRepoCacheType guards the one place a shared read is handed back untyped.
//
// It cannot happen: the singleflight group belongs to this cache alone and the
// function it runs returns T. The assertion is checked rather than bare so that
// if it ever does, the service reports it instead of panicking mid-delivery.
var errRepoCacheType = errors.New("repository cache produced the wrong type")

// repoCache remembers something read per repository for a while.
//
// The Action reads one comment and exits, so it calls the loaders directly. The
// service handles every comment in every repository it serves and sweeps all of
// them on a timer, so without this it would spend a request re-reading files
// that change far less often than it looks at them.
//
// Safe for concurrent use.
type repoCache[T any] struct {
	ttl time.Duration

	// load re-reads a repository's value. previous is what the cache already
	// holds, or nil on the first read, so a loader that can tell cheaply that
	// nothing has changed may hand it straight back.
	//
	// Revalidation lives inside the load rather than beside it because the two
	// share work: the configuration loader asks GitHub one question whose
	// answer both decides whether to re-read and identifies what it read. Split
	// across two hooks, that question was asked twice.
	load func(ctx context.Context, client *github.Client, owner, repo string, previous *T) (T, error)

	// group collapses concurrent misses for one repository into one read.
	group singleflight.Group

	mu      sync.Mutex
	entries map[string]repoCacheEntry[T]
}

type repoCacheEntry[T any] struct {
	value   T
	fetched time.Time
}

func newRepoCache[T any](
	ttl time.Duration,
	load func(ctx context.Context, client *github.Client, owner, repo string, previous *T) (T, error),
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
	return c.GetByKey(ctx, client, bot.RepoFullName(owner, repo), owner, repo)
}

// GetByKey decouples immutable cache identity from mutable GitHub lookup
// coordinates. Service configuration uses the repository ID so a rename
// cannot leave a second, stale configuration entry behind.
func (c *repoCache[T]) GetByKey(
	ctx context.Context,
	client *github.Client,
	key, owner, repo string,
) (T, error) {
	if value, fresh, _ := c.lookup(key); fresh {
		return value, nil
	}

	// One caller refreshes and the rest wait for it. This type's own comment
	// used to claim it avoided the duplicate request and it did not: every
	// caller that missed did the whole read. That was one wasted request each;
	// it is now up to five, because a configuration file is looked for at four
	// paths behind one validator - and a cold start is exactly when every
	// delivery for a repository arrives at once.
	//
	// The shared read runs on the caller's context, so it is cancelled when the
	// work that wanted it is. Detaching it instead - so a cancelled caller does
	// not fail the others - leaves a read running with nothing waiting for it,
	// which is a request outliving the shutdown that was meant to stop it. The
	// cost of not detaching is that abandoning one caller fails the others; they
	// retry, and deliveries are durable.
	shared, err, _ := c.group.Do(key, func() (any, error) {
		return c.refresh(ctx, client, key, owner, repo)
	})
	if err != nil {
		var zero T

		return zero, err
	}

	value, ok := shared.(T)
	if !ok {
		var zero T

		return zero, errRepoCacheType
	}

	return value, nil
}

// refresh re-reads a repository's value, keeping the one it already has when
// that is still provably good.
func (c *repoCache[T]) refresh(
	ctx context.Context,
	client *github.Client,
	key, owner, repo string,
) (T, error) {
	value, fresh, stale := c.lookup(key)
	if fresh {
		// Another caller finished while this one waited for its turn.
		return value, nil
	}

	var previous *T
	if stale {
		previous = &value
	}

	loaded, err := c.load(ctx, client, owner, repo, previous)
	if err != nil {
		// A failure to read is not a reason to serve something Smyklot can no
		// longer vouch for. The caller retries.
		var zero T

		return zero, err
	}

	c.store(key, loaded)

	return loaded, nil
}

// lookup reports a usable entry, or hands back the expired one so the caller
// can ask whether it is still good.
func (c *repoCache[T]) lookup(key string) (value T, fresh, stale bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		var zero T

		return zero, false, false
	}

	return entry.value, time.Since(entry.fetched) < c.ttl, true
}

// store records a value as of now.
//
// There was a generation counter here, so that a load which started earlier but
// finished later could not overwrite a newer one. Nothing can reach that now:
// every store happens inside the singleflight call, which admits one refresh
// per key at a time, so two loads for one repository can no longer overlap at
// all. Keeping both would be two mechanisms for one problem, and the one that
// went is the one that only tidied up after the waste rather than avoiding it.
func (c *repoCache[T]) store(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = repoCacheEntry[T]{value: value, fetched: time.Now()}
}

// newRepoConfigCache builds the cache the service reads a repository's own
// configuration through.
func newRepoConfigCache() *repoCache[repositoryConfigFile] {
	return newRepoCache(repoConfigTTL, fetchRepositoryConfig)
}
