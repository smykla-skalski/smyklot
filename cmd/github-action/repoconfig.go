package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/config"
	"github.com/smykla-skalski/smyklot/pkg/github"
)

// repoConfigCache resolves a repository's effective bot configuration.
//
// A repository states its own preferences in .github/smyklot.yaml, layered over
// whatever the process was started with. The Action reads one comment and
// exits, so it caches nothing; the service handles every comment in every
// repository it serves and would otherwise spend a request on that file each
// time.
type repoConfigCache struct {
	ttl time.Duration

	mu      sync.Mutex
	entries map[string]repoConfigEntry
}

type repoConfigEntry struct {
	cfg     *config.Config
	fetched time.Time
}

// newRepoConfigCache creates a cache. A non-positive ttl disables caching.
func newRepoConfigCache(ttl time.Duration) *repoConfigCache {
	return &repoConfigCache{
		ttl:     ttl,
		entries: make(map[string]repoConfigEntry),
	}
}

// Effective returns base with the repository's own configuration layered over
// it.
//
// A repository without the file gets base back untouched.
func (c *repoConfigCache) Effective(
	ctx context.Context,
	client *github.Client,
	owner, repo string,
	base *config.Config,
) (*config.Config, error) {
	key := fmt.Sprintf("%s/%s", owner, repo)

	if cfg, ok := c.lookup(key); ok {
		return cfg, nil
	}

	content, err := client.GetRepoConfig(ctx, owner, repo)
	if err != nil {
		return nil, NewConfigError(ErrConfigLoad, err)
	}

	cfg, err := config.LoadRepoConfig(base, content)
	if err != nil {
		return nil, NewConfigError(ErrConfigLoad, err)
	}

	c.store(key, cfg)

	return cfg, nil
}

func (c *repoConfigCache) lookup(key string) (*config.Config, bool) {
	if c.ttl <= 0 {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || time.Since(entry.fetched) >= c.ttl {
		return nil, false
	}

	return entry.cfg, true
}

func (c *repoConfigCache) store(key string, cfg *config.Config) {
	if c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = repoConfigEntry{cfg: cfg, fetched: time.Now()}
}
