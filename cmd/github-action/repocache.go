package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/config"
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
	content, err := client.GetRepoConfig(ctx, owner, repo)
	if err != nil {
		return nil, NewConfigError(ErrConfigLoad, err)
	}

	cfg, err := config.LoadRepoConfig(base, content)
	if err != nil {
		return nil, NewConfigError(ErrConfigLoad, err)
	}

	return cfg, nil
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

	mu      sync.Mutex
	entries map[string]repoCacheEntry[T]
}

type repoCacheEntry[T any] struct {
	value   T
	fetched time.Time
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
	key := fmt.Sprintf("%s/%s", owner, repo)

	if value, ok := c.lookup(key); ok {
		return value, nil
	}

	value, err := c.load(ctx, client, owner, repo)
	if err != nil {
		var zero T

		return zero, err
	}

	c.store(key, value)

	return value, nil
}

func (c *repoCache[T]) lookup(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || time.Since(entry.fetched) >= c.ttl {
		var zero T

		return zero, false
	}

	return entry.value, true
}

func (c *repoCache[T]) store(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = repoCacheEntry[T]{value: value, fetched: time.Now()}
}
