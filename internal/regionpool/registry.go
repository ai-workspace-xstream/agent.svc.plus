// Package regionpool contains the provider-neutral runtime model for
// selecting healthy XConnect nodes within a regional pool.
package regionpool

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	LifecycleEnabled  Lifecycle = "enabled"
	LifecycleDisabled Lifecycle = "disabled"
	LifecycleDraining Lifecycle = "draining"
)

var (
	ErrPoolNotFound  = errors.New("region pool not found")
	ErrNodeNotFound  = errors.New("region pool node not found")
	ErrNoHealthyNode = errors.New("no healthy node available")
)

// Lifecycle controls whether a node may receive new selections.
type Lifecycle string

// Heartbeat is the non-sensitive health state reported by a node.
type Heartbeat struct {
	Healthy    bool
	ObservedAt time.Time
	Score      int
	Message    string
}

// Node is the runtime-safe declaration and health state of a pool member. It
// intentionally contains no address or connection credential.
type Node struct {
	ID        string
	Pool      string
	Region    string
	Provider  string
	Product   string
	Weight    int
	Enabled   bool
	Lifecycle Lifecycle
	Heartbeat Heartbeat
}

// Pool describes a stable regional entrypoint and its nodes.
type Pool struct {
	Name       string
	Region     string
	Entrypoint string
	Aliases    []string
	Nodes      []Node
}

// Options controls registry defaults and time handling.
type Options struct {
	HeartbeatTTL  time.Duration
	DefaultWeight int
	Now           func() time.Time
}

// Registry stores pools and provides synchronized lifecycle and selection
// operations.
type Registry struct {
	mu      sync.RWMutex
	pools   map[string]Pool
	aliases map[string]string
	options Options
}

// NewRegistry validates and builds a registry. Aliases map an input pool name
// to a canonical pool name; hp -> ph is always supported unless overridden.
func NewRegistry(pools []Pool, aliases map[string]string, options ...Options) (*Registry, error) {
	opts := Options{HeartbeatTTL: 2 * time.Minute, DefaultWeight: 100, Now: time.Now}
	if len(options) > 0 {
		if options[0].HeartbeatTTL > 0 {
			opts.HeartbeatTTL = options[0].HeartbeatTTL
		}
		if options[0].DefaultWeight > 0 {
			opts.DefaultWeight = options[0].DefaultWeight
		}
		if options[0].Now != nil {
			opts.Now = options[0].Now
		}
	}

	r := &Registry{pools: make(map[string]Pool, len(pools)), aliases: map[string]string{"hp": "ph"}, options: opts}
	for alias, target := range aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		target = strings.ToLower(strings.TrimSpace(target))
		if alias == "" || target == "" {
			return nil, fmt.Errorf("pool alias must have non-empty name and target")
		}
		r.aliases[alias] = target
	}

	regions := make(map[string]string, len(pools))
	allNodes := make(map[string]string)
	for _, pool := range pools {
		pool.Name = strings.ToLower(strings.TrimSpace(pool.Name))
		pool.Region = NormalizeRegion(pool.Region)
		pool.Entrypoint = strings.TrimSpace(pool.Entrypoint)
		if pool.Name == "" || pool.Region == "" || pool.Entrypoint == "" {
			return nil, fmt.Errorf("pool requires name, region, and entrypoint")
		}
		if _, exists := r.pools[pool.Name]; exists {
			return nil, fmt.Errorf("duplicate pool %q", pool.Name)
		}
		if previous, exists := regions[pool.Region]; exists {
			return nil, fmt.Errorf("region %q is declared by pools %q and %q", pool.Region, previous, pool.Name)
		}
		regions[pool.Region] = pool.Name

		seenNodes := make(map[string]struct{}, len(pool.Nodes))
		for i := range pool.Nodes {
			node := &pool.Nodes[i]
			node.ID = strings.TrimSpace(node.ID)
			node.Pool = pool.Name
			node.Region = pool.Region
			node.Provider = strings.TrimSpace(node.Provider)
			node.Product = strings.TrimSpace(node.Product)
			if node.ID == "" {
				return nil, fmt.Errorf("pool %q contains a node with empty id", pool.Name)
			}
			if _, exists := seenNodes[node.ID]; exists {
				return nil, fmt.Errorf("duplicate node %q in pool %q", node.ID, pool.Name)
			}
			if previousPool, exists := allNodes[node.ID]; exists {
				return nil, fmt.Errorf("duplicate node %q in pools %q and %q", node.ID, previousPool, pool.Name)
			}
			seenNodes[node.ID] = struct{}{}
			allNodes[node.ID] = pool.Name
			if node.Weight == 0 {
				node.Weight = opts.DefaultWeight
			}
			if node.Weight < 0 {
				return nil, fmt.Errorf("node %q has invalid weight %d", node.ID, node.Weight)
			}
			if node.Lifecycle == "" {
				node.Lifecycle = LifecycleEnabled
			}
			if !validLifecycle(node.Lifecycle) {
				return nil, fmt.Errorf("node %q has invalid lifecycle %q", node.ID, node.Lifecycle)
			}
			if node.Lifecycle == LifecycleDisabled {
				node.Enabled = false
			}
		}
		r.pools[pool.Name] = pool
	}

	for alias, target := range r.aliases {
		if _, ok := r.pools[target]; !ok {
			return nil, fmt.Errorf("pool alias %q targets unknown pool %q", alias, target)
		}
	}
	return r, nil
}

func validLifecycle(lifecycle Lifecycle) bool {
	return lifecycle == LifecycleEnabled || lifecycle == LifecycleDisabled || lifecycle == LifecycleDraining
}

// NormalizeRegion returns the canonical region name used by pool selection.
func NormalizeRegion(region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "hp" || region == "ph" {
		return "ph-mnl"
	}
	return region
}

func (r *Registry) poolName(regionOrPool string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(regionOrPool))
	if target, ok := r.aliases[key]; ok {
		key = target
	}
	if pool, ok := r.pools[key]; ok {
		return pool.Name, nil
	}
	canonicalRegion := NormalizeRegion(key)
	for name, pool := range r.pools {
		if pool.Region == canonicalRegion {
			return name, nil
		}
		for _, alias := range pool.Aliases {
			if strings.ToLower(strings.TrimSpace(alias)) == key {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("%w: %s", ErrPoolNotFound, regionOrPool)
}

// Nodes returns a snapshot of nodes in the requested pool or region.
func (r *Registry) Nodes(regionOrPool string) []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, err := r.poolName(regionOrPool)
	if err != nil {
		return nil
	}
	pool := r.pools[name]
	return append([]Node(nil), pool.Nodes...)
}

// Entrypoint returns the stable FQDN and whether the requested pool exists.
func (r *Registry) Entrypoint(regionOrPool string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, err := r.poolName(regionOrPool)
	if err != nil {
		return "", false
	}
	return r.pools[name].Entrypoint, true
}

// Heartbeat updates a node's health state and observed time.
func (r *Registry) Heartbeat(nodeID string, heartbeat Heartbeat) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, pool := range r.pools {
		for i := range pool.Nodes {
			if pool.Nodes[i].ID == strings.TrimSpace(nodeID) {
				if heartbeat.ObservedAt.IsZero() {
					heartbeat.ObservedAt = r.options.Now().UTC()
				}
				pool.Nodes[i].Heartbeat = heartbeat
				r.pools[name] = pool
				return nil
			}
		}
	}
	return fmt.Errorf("%w: %s", ErrNodeNotFound, nodeID)
}

func (r *Registry) setLifecycle(nodeID string, lifecycle Lifecycle, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, pool := range r.pools {
		for i := range pool.Nodes {
			if pool.Nodes[i].ID == strings.TrimSpace(nodeID) {
				pool.Nodes[i].Lifecycle = lifecycle
				pool.Nodes[i].Enabled = enabled
				r.pools[name] = pool
				return nil
			}
		}
	}
	return fmt.Errorf("%w: %s", ErrNodeNotFound, nodeID)
}

func (r *Registry) Enable(nodeID string) error { return r.setLifecycle(nodeID, LifecycleEnabled, true) }
func (r *Registry) Disable(nodeID string) error {
	return r.setLifecycle(nodeID, LifecycleDisabled, false)
}
func (r *Registry) Drain(nodeID string) error { return r.setLifecycle(nodeID, LifecycleDraining, true) }
func (r *Registry) Undrain(nodeID string) error {
	return r.setLifecycle(nodeID, LifecycleEnabled, true)
}

// Select chooses a healthy, enabled, non-draining node using stable weighted
// hashing. Node ordering is normalized by ID so results do not depend on YAML
// ordering.
func (r *Registry) Select(regionOrPool, key string) (*Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, err := r.poolName(regionOrPool)
	if err != nil {
		return nil, err
	}
	now := r.options.Now().UTC()
	candidates := make([]Node, 0)
	for _, node := range r.pools[name].Nodes {
		if !node.Enabled || node.Lifecycle != LifecycleEnabled || !node.Heartbeat.Healthy || node.Heartbeat.ObservedAt.IsZero() {
			continue
		}
		if now.Sub(node.Heartbeat.ObservedAt) > r.options.HeartbeatTTL {
			continue
		}
		if node.Weight > 0 {
			candidates = append(candidates, node)
		}
	}
	if len(candidates) == 0 {
		return nil, ErrNoHealthyNode
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	total := uint64(0)
	for _, node := range candidates {
		total += uint64(node.Weight)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	selected := h.Sum64() % total
	for i := range candidates {
		weight := uint64(candidates[i].Weight)
		if selected < weight {
			return &candidates[i], nil
		}
		selected -= weight
	}
	return &candidates[len(candidates)-1], nil
}
