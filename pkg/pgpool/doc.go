// Package pgpool builds pgx connection pools for Drover services and routes
// them through PlanetScale's built-in PgBouncer when that is safe.
//
// Every caller states what kind of connection it needs:
//
//   - Pooled is a normal application pool: short transactions, no session
//     state that outlives a transaction. On a PlanetScale host the port is
//     rewritten from 5432 to 6432 (the local PgBouncer, same credentials).
//   - Session needs a real backend session for its whole lifetime: DBOS's
//     system database (LISTEN/NOTIFY), session-level advisory locks,
//     migrations, DDL. It always connects directly; on a PlanetScale host a
//     6432 port is rewritten back to 5432.
//
// Non-PlanetScale hosts (local dev, Unikraft, a self-hosted Postgres) are
// never rewritten. Every pool gets an explicit MaxConns, never the pgx default
// of max(4, runtime.NumCPU()).
//
// # PlanetScale detection
//
// PlanetScale documents Postgres hostnames as {id+region}.horizon.psdb.cloud
// (https://planetscale.com/docs/postgres/connecting/quickstart) and the local
// PgBouncer as "the same credentials as a direct connection, but port 6432"
// (https://planetscale.com/docs/postgres/connecting/pgbouncer). Connection
// strings can also use a regional endpoint such as
// aws-ap-southeast-2-1.pg.psdb.cloud, which routes by the {role}.{branch_id}
// username; drover's Sydney branch is reached that way. A host matches when it
// ends in ".horizon.psdb.cloud" or ".pg.psdb.cloud" (case-insensitive).
// PlanetScale MySQL hosts (*.connect.psdb.cloud) never match. Private
// connectivity endpoints have other names: add them with WithHostSuffixes or
// the comma-separated DROVER_PG_HOST_SUFFIXES environment variable, or set the
// policy to force.
//
// The policy comes from WithPooler, else the DROVER_PG_POOLER environment
// variable, else auto:
//
//	auto   rewrite only PlanetScale hosts (default)
//	off    never rewrite (kill switch)
//	force  treat every host as PlanetScale, i.e. run PgBouncer on 6432
//
// Only the ports 5432 and 6432 are ever rewritten; any other explicit port is
// left alone.
//
// # MaxConns precedence
//
//  1. WithMaxConns (used as given)
//  2. pool_max_conns in the connection string (used as given)
//  3. the mode default: Pooled 3 (WithDefaultMaxConns is clamped to 2..4),
//     Session 2 (WithDefaultMaxConns is clamped to 1..4)
//
// # Prepared statements
//
// pgx's default exec mode (QueryExecModeCacheStatement) uses protocol-level
// named prepared statements. PlanetScale documents PgBouncer's
// max_prepared_statements as configurable with a default of 200, which makes
// those work in transaction mode
// (https://planetscale.com/docs/postgres/connecting/pgbouncer, "Advanced
// settings"). That is documentation, not a test against our branches, and a
// branch admin can set it to 0. So a Pooled pool that ends up on port 6432
// defaults to QueryExecModeCacheDescribe, which uses unnamed statements and
// works through any PgBouncer. Override with WithExecMode or
// default_query_exec_mode in the connection string.
//
// TODO(pgpool): once max_prepared_statements is confirmed non-zero on the
// production branches, consider QueryExecModeCacheStatement for Pooled pools.
//
// # Edge cases checked in the Drover repos (19 Nov 2026)
//
//   - Row-level security tenant: drover-memory (internal/auth/tenant.go:23)
//     and drover-muster (internal/infra/tenant_scope.go:45) use
//     set_config(..., true), which is transaction-local. Safe on Pooled.
//   - drover-cloud's pg_advisory_xact_lock (internal/recall/store.go:252,360;
//     clients_store.go:160) is transaction-scoped. Safe on Pooled.
//   - Needs Session: DBOS's notification listener (dbos-transact-golang
//     v0.15.0, dbos/system_database.go:2533-2575, holds one connection with
//     LISTEN and WaitForNotification); drover-memory's migration lock
//     (internal/store/migrate.go:34, session pg_advisory_lock); and any pool
//     that relies on named prepared statements (see above).
//   - drover-cloud hands its app pool to DBOS as SystemDBPool
//     (internal/provision/runtime.go:46-49). On adoption it needs two pools:
//     Pooled for the app and Session for DBOS. drover-kelpies likewise.
//
// # Connection budget
//
// PlanetScale's max_connections is 25 on PS-DEV through PS-40. Every Session
// pool connection counts directly against it, and so do PgBouncer's server
// connections (default_pool_size, default 20 per user/database pair). Keep
// Session pools tiny and count replicas: ten DBOS processes with a Session
// pool of 2 already use 20.
package pgpool
