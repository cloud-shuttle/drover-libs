package dbosboot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover-libs/pkg/pgpool"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// noEnv keeps DROVER_PG_POOLER / DROVER_PG_HOST_SUFFIXES on the test host out
// of the results.
func noEnv(string) (string, bool) { return "", false }

func opts(url string) Options {
	return Options{
		AppName:           "drover-test",
		SystemDatabaseURL: url,
		PoolOptions:       []pgpool.Option{pgpool.WithLookupEnv(noEnv)},
	}
}

func TestPoolConfigAlwaysDirect(t *testing.T) {
	cases := []struct {
		name string
		url  string
		port uint16
	}{
		{"planetscale direct stays 5432", "postgres://u:p@abc.horizon.psdb.cloud:5432/db?sslmode=require", 5432},
		{"planetscale pgbouncer rewritten to 5432", "postgres://u:p@abc.horizon.psdb.cloud:6432/db?sslmode=require", 5432},
		{"planetscale regional pgbouncer rewritten to 5432", "postgres://u.b:p@aws-ap-southeast-2-1.pg.psdb.cloud:6432/db?sslmode=require", 5432},
		{"planetscale default port stays 5432", "postgres://u:p@abc.horizon.psdb.cloud/db?sslmode=require", 5432},
		{"other host 6432 untouched", "postgres://u:p@db.internal:6432/db", 6432},
		{"local untouched", "postgres://u:p@localhost:5433/db", 5433},
		{"keyword dsn", "host=abc.horizon.psdb.cloud port=6432 user=u password=p dbname=db sslmode=require", 5432},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := PoolConfig(opts(tc.url))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ConnConfig.Port != tc.port {
				t.Fatalf("port %d, want %d", cfg.ConnConfig.Port, tc.port)
			}
			if cfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeCacheStatement {
				t.Fatalf("exec mode %v, want pgx default (session pools keep prepared statements)", cfg.ConnConfig.DefaultQueryExecMode)
			}
		})
	}
}

func TestPoolConfigForcePolicyStillDirect(t *testing.T) {
	o := opts("postgres://u:p@db.internal:6432/db")
	o.PoolOptions = append(o.PoolOptions, pgpool.WithPooler(pgpool.PolicyForce))
	cfg, err := PoolConfig(o)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Port != 5432 {
		t.Fatalf("port %d, want 5432", cfg.ConnConfig.Port)
	}
}

func TestPoolConfigMaxConns(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		maxConns int32
		want     int32
	}{
		{"default", "postgres://u:p@localhost/db", 0, DefaultMaxConns},
		{"url wins over default", "postgres://u:p@localhost/db?pool_max_conns=5", 0, 5},
		{"explicit", "postgres://u:p@localhost/db", 6, 6},
		{"explicit wins over url", "postgres://u:p@localhost/db?pool_max_conns=9", 4, 4},
		{"explicit floor", "postgres://u:p@localhost/db", 1, MinMaxConns},
		{"negative is default", "postgres://u:p@localhost/db", -1, DefaultMaxConns},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := opts(tc.url)
			o.MaxConns = tc.maxConns
			cfg, err := PoolConfig(o)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MaxConns != tc.want {
				t.Fatalf("MaxConns %d, want %d", cfg.MaxConns, tc.want)
			}
			if cfg.MinConns > cfg.MaxConns {
				t.Fatalf("MinConns %d > MaxConns %d", cfg.MinConns, cfg.MaxConns)
			}
		})
	}
}

func TestPoolConfigTuning(t *testing.T) {
	o := opts("postgres://u:p@localhost/db")
	o.ApplicationName = "drover-test-dbos"
	o.MaxConnLifetime = time.Hour
	o.MaxConnIdleTime = 5 * time.Minute
	o.ConnectTimeout = 10 * time.Second
	cfg, err := PoolConfig(o)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ConnConfig.RuntimeParams["application_name"]; got != "drover-test-dbos" {
		t.Fatalf("application_name %q", got)
	}
	if cfg.MaxConnLifetime != time.Hour || cfg.MaxConnIdleTime != 5*time.Minute {
		t.Fatalf("lifetime %v idle %v", cfg.MaxConnLifetime, cfg.MaxConnIdleTime)
	}
	if cfg.ConnConfig.ConnectTimeout != 10*time.Second {
		t.Fatalf("connect timeout %v", cfg.ConnConfig.ConnectTimeout)
	}
}

func TestPoolConfigURLWinsForAppNameAndTimeout(t *testing.T) {
	o := opts("postgres://u:p@localhost/db?application_name=from-url&connect_timeout=3")
	o.ApplicationName = "ignored"
	o.ConnectTimeout = 10 * time.Second
	cfg, err := PoolConfig(o)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ConnConfig.RuntimeParams["application_name"]; got != "from-url" {
		t.Fatalf("application_name %q, want from-url", got)
	}
	if cfg.ConnConfig.ConnectTimeout != 3*time.Second {
		t.Fatalf("connect timeout %v, want 3s", cfg.ConnConfig.ConnectTimeout)
	}
}

func TestPoolConfigZeroTuningLeavesPgxDefaults(t *testing.T) {
	base, err := PoolConfig(opts("postgres://u:p@localhost/db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := base.ConnConfig.RuntimeParams["application_name"]; ok {
		t.Fatal("application_name set without ApplicationName")
	}
	if base.ConnConfig.ConnectTimeout != 0 {
		t.Fatalf("connect timeout %v, want 0", base.ConnConfig.ConnectTimeout)
	}
}

func TestPoolConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		o    Options
		want string
	}{
		{"no app name", Options{SystemDatabaseURL: "postgres://localhost/db"}, "AppName is required"},
		{"blank url", Options{AppName: "a", SystemDatabaseURL: "  "}, "SystemDatabaseURL is required"},
		{"bad url", Options{AppName: "a", SystemDatabaseURL: "postgres://%zz"}, "system pool"},
		{"bad policy", Options{AppName: "a", SystemDatabaseURL: "postgres://localhost/db", PoolOptions: []pgpool.Option{pgpool.WithPooler("nope")}}, "invalid pooler policy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PoolConfig(tc.o)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v, want %q", err, tc.want)
			}
			if _, _, err := Config(context.Background(), tc.o); err == nil {
				t.Fatal("Config: want error")
			}
		})
	}
}

func TestOpenSystemPoolUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	o := opts("postgres://u:p@127.0.0.1:1/db?connect_timeout=1")
	o.EnsureDatabase = true
	if _, err := OpenSystemPool(ctx, o); err == nil || !strings.Contains(err.Error(), "dbosboot: system pool") {
		t.Fatalf("err %v", err)
	}
}

func TestIsMissingDatabase(t *testing.T) {
	if !isMissingDatabase(fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "3D000"})) {
		t.Fatal("3D000 not detected")
	}
	if isMissingDatabase(&pgconn.PgError{Code: "28P01"}) || isMissingDatabase(errors.New("x")) {
		t.Fatal("false positive")
	}
}

func TestCreateDatabaseNeedsName(t *testing.T) {
	cc, err := pgx.ParseConfig("postgres://u:p@localhost:1")
	if err != nil {
		t.Fatal(err)
	}
	cc.Database = ""
	if err := createDatabase(context.Background(), cc); err == nil {
		t.Fatal("want error")
	}
}

// TestIntegration runs against a real Postgres when DBOSBOOT_TEST_DATABASE_URL
// is set (a superuser URL; the test creates and drops its own database).
func TestIntegration(t *testing.T) {
	base := os.Getenv("DBOSBOOT_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("DBOSBOOT_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cc, err := pgx.ParseConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("dbosboot_test_%d", time.Now().UnixNano())
	cc.Database = name
	url := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", cc.User, cc.Password, cc.Host, cc.Port, name)
	t.Cleanup(func() {
		admin := cc.Copy()
		admin.Database = "postgres"
		if conn, err := pgx.ConnectConfig(context.Background(), admin); err == nil {
			_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
			_ = conn.Close(context.Background())
		}
	})

	o := opts(url)
	if _, err := OpenSystemPool(ctx, o); err == nil || !isMissingDatabase(err) {
		t.Fatalf("without EnsureDatabase: err %v, want 3D000", err)
	}

	o.EnsureDatabase = true
	o.Schema = "dbos_test"
	o.ApplicationName = "dbosboot-test"
	cfg, pool, err := Config(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if cfg.SystemDBPool != pool || cfg.AppName != "drover-test" || cfg.DatabaseSchema != "dbos_test" || cfg.DatabaseURL != "" {
		t.Fatalf("config %+v", cfg)
	}
	if pool.Config().MaxConns != DefaultMaxConns {
		t.Fatalf("MaxConns %d", pool.Config().MaxConns)
	}
	var app string
	if err := pool.QueryRow(ctx, "SELECT current_setting('application_name')").Scan(&app); err != nil || app != "dbosboot-test" {
		t.Fatalf("application_name %q err %v", app, err)
	}

	// DBOS migrates, LISTENs and runs a workflow on the capped Session pool.
	dctx, err := dbos.NewDBOSContext(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	dbos.RegisterWorkflow(dctx, echoWorkflow)
	if err := dbos.Launch(dctx); err != nil {
		t.Fatal(err)
	}
	h, err := dbos.RunWorkflow(dctx, echoWorkflow, "ok")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := h.GetResult(); err != nil || out != "ok" {
		t.Fatalf("workflow result %q err %v", out, err)
	}
	var tables int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'dbos_test'").Scan(&tables); err != nil || tables == 0 {
		t.Fatalf("dbos_test tables %d err %v", tables, err)
	}
	dbos.Shutdown(dctx, 5*time.Second)

	// Second run: database exists, EnsureDatabase is a no-op.
	pool2, err := OpenSystemPool(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	pool2.Close()
	if err := createDatabase(ctx, pool.Config().ConnConfig); err != nil {
		t.Fatalf("createDatabase on existing db: %v", err)
	}
}

func echoWorkflow(ctx dbos.DBOSContext, in string) (string, error) {
	return dbos.RunAsStep(ctx, func(context.Context) (string, error) { return in, nil })
}
