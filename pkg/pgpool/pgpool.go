package pgpool

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Mode says what kind of connection a pool needs.
type Mode int

const (
	// Pooled is a normal application pool; it may go through PgBouncer.
	Pooled Mode = iota + 1
	// Session needs a direct backend session (LISTEN, session locks, DDL).
	Session
)

// String returns "pooled" or "session".
func (m Mode) String() string {
	switch m {
	case Pooled:
		return "pooled"
	case Session:
		return "session"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

// Ports used by PlanetScale Postgres.
const (
	DirectPort uint16 = 5432
	PoolerPort uint16 = 6432
)

// Default and clamp values for MaxConns.
const (
	DefaultPooledMaxConns  int32 = 3
	DefaultSessionMaxConns int32 = 2
	minPooledMaxConns      int32 = 2
	minSessionMaxConns     int32 = 1
	maxDefaultMaxConns     int32 = 4
)

// PlanetScaleHostSuffix is the documented per-database PlanetScale Postgres
// host suffix ({id+region}.horizon.psdb.cloud).
const PlanetScaleHostSuffix = ".horizon.psdb.cloud"

// PlanetScaleRegionalHostSuffix is the suffix of PlanetScale's regional
// Postgres endpoints ({provider}-{region}-{n}.pg.psdb.cloud, for example
// aws-ap-southeast-2-1.pg.psdb.cloud), which route by the {role}.{branch_id}
// username and serve PgBouncer on 6432 like the per-database hosts.
const PlanetScaleRegionalHostSuffix = ".pg.psdb.cloud"

// PlanetScaleHostSuffixes are the host suffixes auto detection matches.
// PlanetScale MySQL hosts (*.connect.psdb.cloud) are deliberately excluded.
var PlanetScaleHostSuffixes = []string{PlanetScaleHostSuffix, PlanetScaleRegionalHostSuffix}

// New parses databaseURL, applies the mode's routing and limits, and opens a
// pool. databaseURL may be a postgres:// URL or a key=value DSN.
func New(ctx context.Context, databaseURL string, mode Mode, opts ...Option) (*pgxpool.Pool, error) {
	cfg, err := Config(databaseURL, mode, opts...)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Config is New without opening the pool, for callers that need to adjust
// the *pgxpool.Config (hooks, tracers) before pgxpool.NewWithConfig.
func Config(databaseURL string, mode Mode, opts ...Option) (*pgxpool.Config, error) {
	if mode != Pooled && mode != Session {
		return nil, fmt.Errorf("pgpool: unknown mode %d", int(mode))
	}
	o, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgpool: parse connection string: %w", err)
	}
	params := connParams(databaseURL)

	route(cfg.ConnConfig, mode, o)
	cfg.MaxConns = maxConns(mode, o, params, cfg.MaxConns)
	if cfg.MinConns > cfg.MaxConns {
		cfg.MinConns = cfg.MaxConns
	}
	setExecMode(cfg.ConnConfig, mode, o, params)
	return cfg, nil
}

// route rewrites the port of the primary host and every fallback.
func route(cc *pgx.ConnConfig, mode Mode, o options) {
	cc.Port = targetPort(cc.Host, cc.Port, mode, o)
	for _, fb := range cc.Fallbacks {
		fb.Port = targetPort(fb.Host, fb.Port, mode, o)
	}
}

// targetPort returns the port a host should use for mode.
func targetPort(host string, port uint16, mode Mode, o options) uint16 {
	if !o.isPlanetScale(host) {
		return port
	}
	if mode == Pooled && port == DirectPort {
		return PoolerPort
	}
	if mode == Session && port == PoolerPort {
		return DirectPort
	}
	return port
}

func maxConns(mode Mode, o options, params map[string]bool, parsed int32) int32 {
	if o.maxConns > 0 {
		return o.maxConns
	}
	if params["pool_max_conns"] {
		return parsed
	}
	if mode == Session {
		return clamp(o.defaultMaxConns, DefaultSessionMaxConns, minSessionMaxConns)
	}
	return clamp(o.defaultMaxConns, DefaultPooledMaxConns, minPooledMaxConns)
}

func clamp(v, def, lo int32) int32 {
	if v == 0 {
		return def
	}
	return min(max(v, lo), maxDefaultMaxConns)
}

// setExecMode picks a PgBouncer-safe exec mode for Pooled pools on 6432.
func setExecMode(cc *pgx.ConnConfig, mode Mode, o options, params map[string]bool) {
	if o.execModeSet {
		cc.DefaultQueryExecMode = o.execMode
		return
	}
	if params["default_query_exec_mode"] || mode != Pooled || !onPooler(cc) {
		return
	}
	cc.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
}

func onPooler(cc *pgx.ConnConfig) bool {
	if cc.Port == PoolerPort {
		return true
	}
	return anyFallbackOnPooler(cc.Fallbacks)
}

func anyFallbackOnPooler(fbs []*pgconn.FallbackConfig) bool {
	for _, fb := range fbs {
		if fb.Port == PoolerPort {
			return true
		}
	}
	return false
}
