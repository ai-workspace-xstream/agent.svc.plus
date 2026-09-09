package xrayconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ClientSource provides the list of active Xray clients to encode in the config.
type ClientSource interface {
	ListClients(ctx context.Context) ([]Client, error)
}

type commandRunner func(ctx context.Context, cmd []string) ([]byte, error)

// UserAdder adds clients to a running Xray instance without restarting it.
// It is deliberately addition-only: removing a user from Xray's validator does
// not terminate that user's established sessions.
type UserAdder interface {
	AddUsers(ctx context.Context, generator Generator, clients []Client) error
}

// PeriodicOptions configures a PeriodicSyncer instance.
type PeriodicOptions struct {
	Logger          *slog.Logger
	Interval        time.Duration
	Source          ClientSource
	Generator       Generator
	ValidateCommand []string
	RestartCommand  []string
	Runner          commandRunner
	UserAdder       UserAdder
	OnSync          func(SyncResult)
}

// PeriodicSyncer periodically rebuilds the Xray configuration from the database.
type PeriodicSyncer struct {
	logger          *slog.Logger
	interval        time.Duration
	source          ClientSource
	generator       Generator
	validateCommand []string
	restartCommand  []string
	runner          commandRunner
	userAdder       UserAdder
	onSync          func(SyncResult)
	trigger         chan struct{}

	mu          sync.Mutex
	lastHash    string
	lastClients []Client
}

// SyncResult describes the outcome of a synchronization attempt.
type SyncResult struct {
	Clients     int
	Error       error
	CompletedAt time.Time
	Changed     bool
}

// NewPeriodicSyncer constructs a new PeriodicSyncer from the provided options.
func NewPeriodicSyncer(opts PeriodicOptions) (*PeriodicSyncer, error) {
	if opts.Source == nil {
		return nil, errors.New("client source is required")
	}
	if strings.TrimSpace(opts.Generator.OutputPath) == "" {
		return nil, errors.New("generator output path is required")
	}
	if opts.Interval <= 0 {
		return nil, errors.New("interval must be positive")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	runner := opts.Runner
	if runner == nil {
		runner = defaultCommandRunner
	}
	return &PeriodicSyncer{
		logger:          logger,
		interval:        opts.Interval,
		source:          opts.Source,
		generator:       opts.Generator,
		validateCommand: append([]string(nil), opts.ValidateCommand...),
		restartCommand:  append([]string(nil), opts.RestartCommand...),
		runner:          runner,
		userAdder:       opts.UserAdder,
		onSync:          opts.OnSync,
		trigger:         make(chan struct{}, 1),
	}, nil
}

// Trigger requests an immediate reconciliation. Multiple pending events are
// coalesced because every reconciliation fetches the complete desired state.
func (s *PeriodicSyncer) Trigger() {
	if s == nil {
		return
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Start launches the synchronization loop. The returned stop function cancels the
// loop and waits for it to exit, honouring the provided context for the wait.
func (s *PeriodicSyncer) Start(ctx context.Context) (func(context.Context) error, error) {
	if s == nil {
		return nil, errors.New("syncer is nil")
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.run(runCtx)
	}()
	stop := func(waitCtx context.Context) error {
		cancel()
		if waitCtx == nil {
			waitCtx = context.Background()
		}
		select {
		case <-done:
			return nil
		case <-waitCtx.Done():
			return waitCtx.Err()
		}
	}
	return stop, nil
}

func (s *PeriodicSyncer) run(ctx context.Context) {
	if n, err := s.sync(ctx); err != nil {
		s.notify(SyncResult{Clients: n, Error: err, CompletedAt: time.Now().UTC()})
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			s.logger.Error("xray config sync failed", "err", err)
		}
		if ctx.Err() != nil {
			return
		}
	} else {
		s.logger.Info("xray config synchronized", "clients", n)
		s.notify(SyncResult{Clients: n, CompletedAt: time.Now().UTC()})
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.trigger:
			s.logger.Info("xray config sync triggered by controller event")
		}
		{
			n, err := s.sync(ctx)
			if err != nil {
				s.notify(SyncResult{Clients: n, Error: err, CompletedAt: time.Now().UTC()})
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				s.logger.Error("xray config sync failed", "err", err)
				continue
			}
			s.logger.Info("xray config synchronized", "clients", n)
			s.notify(SyncResult{Clients: n, CompletedAt: time.Now().UTC()})
		}
	}
}

func (s *PeriodicSyncer) sync(ctx context.Context) (int, error) {
	clients, err := s.source.ListClients(ctx)
	if err != nil {
		return 0, fmt.Errorf("list clients: %w", err)
	}

	renderedBuf, err := s.generator.Render(clients)
	if err != nil {
		return 0, fmt.Errorf("render config: %w", err)
	}

	currentHash := sha256Hex(renderedBuf)

	s.mu.Lock()
	previousHash := s.lastHash
	s.mu.Unlock()

	if previousHash != "" && currentHash == previousHash {
		s.logger.Info("xray config unchanged, skipping write and restart", "clients", len(clients), "hash", currentHash[:12])
		s.notify(SyncResult{Clients: len(clients), Changed: false, CompletedAt: time.Now().UTC()})
		return len(clients), nil
	}

	if err := s.generator.Generate(clients); err != nil {
		return 0, fmt.Errorf("generate config: %w", err)
	}

	if len(s.validateCommand) > 0 {
		if err := s.runCommand(ctx, s.validateCommand, "validate config"); err != nil {
			return 0, err
		}
	}

	// The first sync bootstraps the API/tagged inbound through the normal
	// restart path. After that, pure additions can be applied through
	// HandlerService without interrupting established connections. Withdrawing a
	// paused user's node-local credential or mutating a credential still restarts
	// Xray to enforce the pause immediately; this never deletes the account.
	s.mu.Lock()
	previousClients := append([]Client(nil), s.lastClients...)
	s.mu.Unlock()
	added, destructive := clientDelta(previousClients, clients)
	dynamicApplied := false
	if previousHash != "" && !destructive {
		if len(added) == 0 {
			dynamicApplied = true // Same client set; only ordering changed.
		} else if s.userAdder != nil {
			if err := s.userAdder.AddUsers(ctx, s.generator, added); err != nil {
				s.logger.Warn("dynamic xray user add failed; falling back to restart", "err", err, "users", len(added))
			} else {
				dynamicApplied = true
				s.logger.Info("xray users added without restart", "users", len(added))
			}
		}
	}
	if !dynamicApplied && len(s.restartCommand) > 0 {
		if err := s.runCommand(ctx, s.restartCommand, "restart xray"); err != nil {
			return 0, err
		}
	} else if !dynamicApplied && s.userAdder != nil && previousHash != "" {
		return 0, errors.New("dynamic user update failed and no restart command is configured")
	}

	s.mu.Lock()
	s.lastHash = currentHash
	s.lastClients = append([]Client(nil), clients...)
	s.mu.Unlock()

	return len(clients), nil
}

func clientDelta(previous, current []Client) (added []Client, destructive bool) {
	previousByID := make(map[string]Client, len(previous))
	currentByID := make(map[string]Client, len(current))
	for _, client := range previous {
		previousByID[client.ID] = client
	}
	for _, client := range current {
		currentByID[client.ID] = client
		old, exists := previousByID[client.ID]
		if !exists {
			added = append(added, client)
			continue
		}
		if old.Email != client.Email || old.Flow != client.Flow {
			destructive = true
		}
	}
	for id := range previousByID {
		if _, exists := currentByID[id]; !exists {
			destructive = true
		}
	}
	return added, destructive
}

func (s *PeriodicSyncer) notify(result SyncResult) {
	if s.onSync == nil {
		return
	}
	s.onSync(result)
}

func (s *PeriodicSyncer) runCommand(ctx context.Context, cmd []string, action string) error {
	output, err := s.runner(ctx, cmd)
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%s: %w: %s", action, err, strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("%s: %w", action, err)
	}
	if len(output) > 0 {
		s.logger.Debug(action, "output", strings.TrimSpace(string(output)))
	}
	return nil
}

func defaultCommandRunner(ctx context.Context, cmd []string) ([]byte, error) {
	if len(cmd) == 0 {
		return nil, errors.New("command is empty")
	}
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	return c.CombinedOutput()
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
