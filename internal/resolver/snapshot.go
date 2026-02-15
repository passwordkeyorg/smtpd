package resolver

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type DomainMode string

const (
	ModeAllowlist DomainMode = "allowlist"
	ModeCatchAll  DomainMode = "catch_all"
)

type Snapshot struct {
	GeneratedAt time.Time               `json:"generated_at"`
	Domains     map[string]DomainConfig `json:"domains"`
}

type DomainConfig struct {
	Mode         DomainMode `json:"mode"`
	PlusTag      bool       `json:"plus_tag"`
	Mailboxes    []string   `json:"mailboxes"`
	Disabled     bool       `json:"disabled"`
	MaxMsgBytes  int64      `json:"max_msg_bytes"`
	MaxRcptCount int        `json:"max_rcpt_count"`
}

type ResolverStats struct {
	Domains   int
	Mailboxes int
}

type Resolver interface {
	Allowed(domain, local string) bool
	Stats() ResolverStats
}

type allowlistResolver struct {
	domains map[string]domainAllow
	stats   ResolverStats
}

type domainAllow struct {
	plusTag bool
	set     map[string]struct{}
}

func (r *allowlistResolver) Allowed(domain, local string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	cfg, ok := r.domains[d]
	if !ok {
		return false
	}
	if _, ok := cfg.set[local]; ok {
		return true
	}
	if cfg.plusTag {
		base, _, ok := strings.Cut(local, "+")
		if ok {
			_, ok2 := cfg.set[base]
			return ok2
		}
	}
	return false
}

func (r *allowlistResolver) Stats() ResolverStats { return r.stats }

func BuildAllowlist(s Snapshot) (Resolver, error) {
	domains := make(map[string]domainAllow, len(s.Domains))
	stats := ResolverStats{}
	for dom, cfg := range s.Domains {
		d := strings.ToLower(strings.TrimSuffix(dom, "."))
		if cfg.Disabled {
			continue
		}
		if cfg.Mode != "" && cfg.Mode != ModeAllowlist {
			return nil, fmt.Errorf("domain %s: unsupported mode %q", dom, cfg.Mode)
		}
		set := make(map[string]struct{}, len(cfg.Mailboxes))
		for _, mb := range cfg.Mailboxes {
			m := strings.ToLower(mb)
			set[m] = struct{}{}
		}
		domains[d] = domainAllow{plusTag: cfg.PlusTag, set: set}
		stats.Domains++
		stats.Mailboxes += len(set)
	}
	return &allowlistResolver{domains: domains, stats: stats}, nil
}

type Loader struct {
	Path string
	cur  atomic.Pointer[allowlistResolver]
}

// Loader implements Resolver by delegating to the latest loaded snapshot.
func (l *Loader) Allowed(domain, local string) bool { return l.Current().Allowed(domain, local) }
func (l *Loader) Stats() ResolverStats              { return l.Current().Stats() }

func (l *Loader) Load() (Resolver, error) {
	b, err := os.ReadFile(l.Path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	res, err := BuildAllowlist(snap)
	if err != nil {
		return nil, err
	}
	l.cur.Store(res.(*allowlistResolver))
	return res, nil
}

func (l *Loader) Current() Resolver {
	p := l.cur.Load()
	if p == nil {
		return &allowlistResolver{domains: map[string]domainAllow{}, stats: ResolverStats{}}
	}
	return p
}
