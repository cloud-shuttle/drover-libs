package pgpool

import (
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
)

// EnvPooler is the environment variable that sets the pooler policy.
const EnvPooler = "DROVER_PG_POOLER"

// Policy controls PlanetScale detection.
type Policy string

const (
	// PolicyAuto rewrites only PlanetScale hosts.
	PolicyAuto Policy = "auto"
	// PolicyOff never rewrites (kill switch).
	PolicyOff Policy = "off"
	// PolicyForce treats every host as PlanetScale.
	PolicyForce Policy = "force"
)

// Option configures Config and New.
type Option func(*options)

type options struct {
	policy          Policy
	policySet       bool
	suffixes        []string
	maxConns        int32
	defaultMaxConns int32
	execMode        pgx.QueryExecMode
	execModeSet     bool
	lookupEnv       func(string) (string, bool)
}

// WithPooler sets the policy, overriding DROVER_PG_POOLER.
func WithPooler(p Policy) Option {
	return func(o *options) { o.policy, o.policySet = p, true }
}

// WithHostSuffixes adds host suffixes (or exact hosts) to treat as
// PlanetScale, for example private connectivity endpoints.
func WithHostSuffixes(suffixes ...string) Option {
	return func(o *options) { o.suffixes = append(o.suffixes, suffixes...) }
}

// WithMaxConns sets MaxConns exactly, overriding pool_max_conns and the mode
// default. Values below 1 are ignored.
func WithMaxConns(n int32) Option {
	return func(o *options) {
		if n > 0 {
			o.maxConns = n
		}
	}
}

// WithDefaultMaxConns changes the mode default, clamped to 2..4 for Pooled
// and 1..4 for Session. pool_max_conns in the URL still wins.
func WithDefaultMaxConns(n int32) Option {
	return func(o *options) { o.defaultMaxConns = n }
}

// WithExecMode sets pgx's DefaultQueryExecMode, overriding the connection
// string and the Pooled default.
func WithExecMode(m pgx.QueryExecMode) Option {
	return func(o *options) { o.execMode, o.execModeSet = m, true }
}

// WithLookupEnv replaces os.LookupEnv, for tests.
func WithLookupEnv(f func(string) (string, bool)) Option {
	return func(o *options) { o.lookupEnv = f }
}

func resolveOptions(opts []Option) (options, error) {
	o := options{lookupEnv: os.LookupEnv, suffixes: []string{PlanetScaleHostSuffix}}
	for _, opt := range opts {
		opt(&o)
	}
	if !o.policySet {
		o.policy = PolicyAuto
		if v, ok := o.lookupEnv(EnvPooler); ok && strings.TrimSpace(v) != "" {
			o.policy = Policy(strings.ToLower(strings.TrimSpace(v)))
		}
	}
	switch o.policy {
	case PolicyAuto, PolicyOff, PolicyForce:
		return o, nil
	}
	return o, fmt.Errorf("pgpool: invalid pooler policy %q (want auto, off or force)", o.policy)
}

// IsPlanetScaleHost reports whether host matches the documented PlanetScale
// Postgres host suffix.
func IsPlanetScaleHost(host string) bool {
	return hasSuffix(host, []string{PlanetScaleHostSuffix})
}

func (o options) isPlanetScale(host string) bool {
	switch o.policy {
	case PolicyOff:
		return false
	case PolicyForce:
		return true
	}
	return hasSuffix(host, o.suffixes)
}

func hasSuffix(host string, suffixes []string) bool {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	for _, s := range suffixes {
		s = strings.TrimSuffix(strings.ToLower(s), ".")
		if s != "" && (h == strings.TrimPrefix(s, ".") || strings.HasSuffix(h, "."+strings.TrimPrefix(s, "."))) {
			return true
		}
	}
	return false
}
