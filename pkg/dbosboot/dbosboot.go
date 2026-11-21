// Package dbosboot builds the DBOS system-database pool and dbos.Config the
// same way in every Drover DBOS runtime owner (drover-cloud, drover-kelpies,
// the drover orchestrator CLI).
//
// It only does plumbing. Workflow registration, Launch and Shutdown stay in
// the owning repo (Brain ADR 0007 §6), because DBOS requires every workflow
// to be registered between dbos.NewDBOSContext and dbos.Launch.
//
// The system pool is always a pgpool.Session pool:
//
//   - DBOS holds one connection for LISTEN/WaitForNotification for the life of
//     the process and runs its schema migrations on the same pool. Neither
//     works through PlanetScale's transaction-mode PgBouncer, so on a
//     PlanetScale host a 6432 port is rewritten back to 5432 (direct). This is
//     the inverse of pgpool's Pooled rewrite to 6432; other hosts are never
//     rewritten. DROVER_PG_POOLER and DROVER_PG_HOST_SUFFIXES apply as in
//     pgpool.
//   - The pool is capped (DefaultMaxConns, 3) instead of DBOS's own default of
//     20, which PlanetScale's small tiers cannot afford. One connection is
//     held by the listener, so an explicit cap is never below MinMaxConns (2).
//
// Typical use:
//
//	cfg, pool, err := dbosboot.Config(ctx, dbosboot.Options{
//		AppName:           "drover-cloud",
//		SystemDatabaseURL: os.Getenv("DBOS_SYSTEM_DATABASE_URL"),
//	})
//	if err != nil { ... }
//	dbosCtx, err := dbos.NewDBOSContext(ctx, cfg)
//	if err != nil { pool.Close(); ... }
//	// register workflows, then dbos.Launch(dbosCtx)
//	// on exit: dbos.Shutdown(dbosCtx, timeout); pool.Close()
package dbosboot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover-libs/pkg/pgpool"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvSystemDatabaseURL is the environment variable every Drover DBOS runtime
// reads its system-database URL from. dbosboot never reads it itself; callers
// pass the value in Options.SystemDatabaseURL.
const EnvSystemDatabaseURL = "DBOS_SYSTEM_DATABASE_URL"

const (
	// DefaultMaxConns is the system-pool cap when Options.MaxConns is 0: one
	// connection for the LISTEN loop leaves two for workflow status, step
	// checkpoints and queue polling.
	DefaultMaxConns int32 = 3
	// MinMaxConns is the floor for an explicit Options.MaxConns, because the
	// notification listener holds one connection permanently.
	MinMaxConns int32 = 2
)

// Options configures the system pool and dbos.Config.
type Options struct {
	// AppName is dbos.Config.AppName. Required.
	AppName string
	// SystemDatabaseURL is the DBOS system database, a postgres:// URL or a
	// key=value DSN. Required. On PlanetScale it is always used on 5432.
	SystemDatabaseURL string
	// Schema is dbos.Config.DatabaseSchema. Empty means the DBOS default
	// ("dbos"). Set it when two DBOS apps share one database.
	Schema string

	// MaxConns, when > 0, sets the cap exactly, overriding pool_max_conns in
	// the URL; values below MinMaxConns are raised to MinMaxConns. When 0 the
	// cap is DefaultMaxConns and pool_max_conns in the URL still wins.
	MaxConns int32

	// ApplicationName sets the Postgres application_name unless the URL
	// already sets one. Empty leaves it unset.
	ApplicationName string
	// MaxConnLifetime and MaxConnIdleTime override pgx's defaults when > 0.
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	// ConnectTimeout applies when > 0 and the URL has no connect_timeout.
	ConnectTimeout time.Duration

	// EnsureDatabase creates the system database (via the "postgres"
	// maintenance database, on the same direct connection settings) when the
	// first ping reports it does not exist. This mirrors what DBOS does on its
	// own when it is not handed a pool. Leave it off for managed Postgres
	// (PlanetScale roles cannot use the maintenance database).
	EnsureDatabase bool

	// PoolOptions are passed to pgpool.Config after dbosboot's own options,
	// for example pgpool.WithHostSuffixes or pgpool.WithLookupEnv in tests.
	PoolOptions []pgpool.Option
}

func (o Options) validate() error {
	if strings.TrimSpace(o.AppName) == "" {
		return errors.New("dbosboot: AppName is required")
	}
	if strings.TrimSpace(o.SystemDatabaseURL) == "" {
		return errors.New("dbosboot: SystemDatabaseURL is required")
	}
	return nil
}

// PoolConfig returns the system pool config without opening it.
func PoolConfig(o Options) (*pgxpool.Config, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	popts := []pgpool.Option{pgpool.WithDefaultMaxConns(DefaultMaxConns)}
	if o.MaxConns > 0 {
		popts = append(popts, pgpool.WithMaxConns(max(o.MaxConns, MinMaxConns)))
	}
	popts = append(popts, o.PoolOptions...)
	cfg, err := pgpool.Config(o.SystemDatabaseURL, pgpool.Session, popts...)
	if err != nil {
		return nil, fmt.Errorf("dbosboot: system pool: %w", err)
	}
	if o.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = o.MaxConnLifetime
	}
	if o.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = o.MaxConnIdleTime
	}
	if o.ConnectTimeout > 0 && cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = o.ConnectTimeout
	}
	if o.ApplicationName != "" {
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		if cfg.ConnConfig.RuntimeParams["application_name"] == "" {
			cfg.ConnConfig.RuntimeParams["application_name"] = o.ApplicationName
		}
	}
	return cfg, nil
}

// OpenSystemPool opens and pings the system pool (see PoolConfig).
func OpenSystemPool(ctx context.Context, o Options) (*pgxpool.Pool, error) {
	cfg, err := PoolConfig(o)
	if err != nil {
		return nil, err
	}
	pool, err := open(ctx, cfg)
	if err != nil && o.EnsureDatabase && isMissingDatabase(err) {
		if cerr := createDatabase(ctx, cfg.ConnConfig); cerr != nil {
			return nil, fmt.Errorf("dbosboot: create system database: %w", cerr)
		}
		pool, err = open(ctx, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("dbosboot: system pool: %w", err)
	}
	return pool, nil
}

// Config opens the system pool and returns a dbos.Config that uses it. The
// caller owns the pool: close it after dbos.Shutdown, or straight away if
// dbos.NewDBOSContext fails.
//
// DatabaseURL is deliberately left empty: DBOS ignores it when SystemDBPool
// is set, and leaving it out keeps a second copy of the credentials out of
// the config.
func Config(ctx context.Context, o Options) (dbos.Config, *pgxpool.Pool, error) {
	pool, err := OpenSystemPool(ctx, o)
	if err != nil {
		return dbos.Config{}, nil, err
	}
	return dbos.Config{
		AppName:        o.AppName,
		SystemDBPool:   pool,
		DatabaseSchema: strings.TrimSpace(o.Schema),
	}, pool, nil
}

func open(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// isMissingDatabase reports SQLSTATE 3D000 (invalid_catalog_name).
func isMissingDatabase(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "3D000"
}

func createDatabase(ctx context.Context, cc *pgx.ConnConfig) error {
	name := cc.Database
	if name == "" {
		return errors.New("no database name in the connection string")
	}
	server := cc.Copy()
	server.Database = "postgres"
	conn, err := pgx.ConnectConfig(ctx, server)
	if err != nil {
		return fmt.Errorf("connect to maintenance database: %w", err)
	}
	defer conn.Close(ctx)
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return fmt.Errorf("check database %q: %w", name, err)
	}
	if exists {
		return nil
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return fmt.Errorf("create database %q: %w", name, err)
	}
	return nil
}
