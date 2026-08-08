package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/smykla-skalski/smyklot/pkg/github"
)

// startWorkers launches the pool that executes queued deliveries.
func (s *server) startWorkers() *sync.WaitGroup {
	var workers sync.WaitGroup

	for range workerCount {
		workers.Add(1)

		go func() {
			defer workers.Done()

			for j := range s.jobs {
				s.execute(j)
			}
		}()
	}

	return &workers
}

// drain waits for queued deliveries to finish, giving up rather than blocking a
// shutdown forever.
func (s *server) drain(workers *sync.WaitGroup) {
	drained := make(chan struct{})

	go func() {
		workers.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(drainTimeout):
		log.Print("gave up waiting for in-flight deliveries")
	}
}

// pollLoop sweeps on an interval until ctx is cancelled.
//
// GitHub delivers no webhook when someone adds or removes a reaction, so
// reaction commands can only be found by looking. The same sweep merges pull
// requests that were waiting for CI.
//
// Sweeping in the loop rather than in a goroutine per tick means a sweep that
// outruns the interval delays the next one instead of overlapping with it.
func (s *server) pollLoop(ctx context.Context) {
	if s.cfg.pollInterval <= 0 {
		log.Print("reaction polling disabled")

		return
	}

	log.Printf("sweeping reactions every %s", s.cfg.pollInterval)

	ticker := time.NewTicker(s.cfg.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if err := s.sweep(ctx); err != nil {
				log.Printf("sweep failed: %v", err)
			}
		}
	}
}

// sweep polls every repository the App is installed on.
//
// The installation list comes from GitHub rather than from configuration, so a
// repository installed while the process runs is swept on the next tick without
// a restart.
func (s *server) sweep(ctx context.Context) error {
	appToken, err := s.tokens.AppToken()
	if err != nil {
		return NewGitHubError(ErrGitHubAppAuth, err)
	}

	appClient, err := github.NewAppClient(appToken, s.cfg.apiBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	installations, err := appClient.ListInstallations(ctx)
	if err != nil {
		return NewGitHubError(ErrListInstallations, err)
	}

	for _, installation := range installations {
		// One installation losing access must not stop the rest of the sweep
		if err := s.sweepInstallation(ctx, installation); err != nil {
			log.Printf("installation %d (%s): %v", installation.ID, installation.Account, err)
		}
	}

	return nil
}

// sweepInstallation polls every repository one installation can reach.
func (s *server) sweepInstallation(ctx context.Context, installation github.Installation) error {
	token, err := s.tokens.InstallationToken(installation.ID)
	if err != nil {
		return NewGitHubError(ErrGitHubAppAuth, err)
	}

	client, err := github.NewClient(token, s.cfg.apiBaseURL)
	if err != nil {
		return NewGitHubError(ErrGitHubClient, err)
	}

	repos, err := client.ListInstallationRepos(ctx)
	if err != nil {
		return NewGitHubError(ErrListRepos, err)
	}

	for _, repo := range repos {
		if err := s.sweepRepo(ctx, client, repo); err != nil {
			log.Printf("%s/%s: %v", repo.Owner, repo.Name, err)
		}
	}

	return nil
}

// sweepRepo polls one repository, using the same code the poll subcommand runs.
//
// Both files it needs are cached: a sweep would otherwise re-read every
// repository's CODEOWNERS and config on every tick, forever, for content that
// changes far less often than it is looked at.
func (s *server) sweepRepo(ctx context.Context, client *github.Client, repo github.Repository) error {
	bc, err := s.configs.Get(ctx, client, repo.Owner, repo.Name)
	if err != nil {
		return err
	}

	codeowners, err := s.owners.Get(ctx, client, repo.Owner, repo.Name)
	if err != nil {
		return err
	}

	// The checker is built fresh rather than cached with the content, so it
	// always holds the client carrying the current installation token
	checker, err := checkerFromCodeowners(codeowners, client)
	if err != nil {
		return err
	}

	return pollAllPRs(ctx, client, checker, bc, repo.Owner, repo.Name, s.cfg.botUsername)
}
