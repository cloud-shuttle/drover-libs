package pgpool

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	psURL   = "postgres://app.abc123:pscale_pw_x@abc123-apsoutheast2-1.horizon.psdb.cloud:5432/drover?sslmode=require"
	psDSN   = "host=abc123-apsoutheast2-1.horizon.psdb.cloud port=5432 user=app.abc123 password='p w\\'x' dbname=drover sslmode=require"
	localDB = "postgres://drover:drover@localhost:5432/drover?sslmode=disable"
)

// noEnv keeps tests independent of the caller's DROVER_PG_POOLER.
func noEnv() Option {
	return WithLookupEnv(func(string) (string, bool) { return "", false })
}

func env(v string) Option {
	return WithLookupEnv(func(k string) (string, bool) {
		if k == EnvPooler {
			return v, true
		}
		return "", false
	})
}

func mustConfig(t *testing.T, s string, m Mode, opts ...Option) (uint16, int32, pgx.QueryExecMode) {
	t.Helper()
	cfg, err := Config(s, m, append([]Option{noEnv()}, opts...)...)
	if err != nil {
		t.Fatalf("Config(%q, %v): %v", s, m, err)
	}
	return cfg.ConnConfig.Port, cfg.MaxConns, cfg.ConnConfig.DefaultQueryExecMode
}

func TestRouting(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		mode Mode
		opts []Option
		port uint16
	}{
		{"pooled planetscale url", psURL, Pooled, nil, PoolerPort},
		{"pooled planetscale dsn", psDSN, Pooled, nil, PoolerPort},
		{"pooled planetscale no port", "postgres://u:p@abc-useast1-1.horizon.psdb.cloud/db", Pooled, nil, PoolerPort},
		{"pooled planetscale already 6432", strings.Replace(psURL, ":5432", ":6432", 1), Pooled, nil, PoolerPort},
		{"pooled planetscale custom port", strings.Replace(psURL, ":5432", ":7000", 1), Pooled, nil, 7000},
		{"pooled planetscale upper case trailing dot", "postgres://u:p@ABC-USEAST1-1.Horizon.PSDB.cloud.:5432/db", Pooled, nil, PoolerPort},
		{"session planetscale", psURL, Session, nil, DirectPort},
		{"session planetscale given 6432", strings.Replace(psURL, ":5432", ":6432", 1), Session, nil, DirectPort},
		{"pooled local", localDB, Pooled, nil, DirectPort},
		{"pooled lookalike host", "postgres://u:p@horizon.psdb.cloud.evil.example:5432/db", Pooled, nil, DirectPort},
		{"pooled mysql planetscale host", "postgres://u:p@aws.connect.psdb.cloud:5432/db", Pooled, nil, DirectPort},
		{"policy off", psURL, Pooled, []Option{WithPooler(PolicyOff)}, DirectPort},
		{"policy force local", localDB, Pooled, []Option{WithPooler(PolicyForce)}, PoolerPort},
		{"extra suffix", "postgres://u:p@db.vpce.internal:5432/db", Pooled, []Option{WithHostSuffixes(".vpce.internal")}, PoolerPort},
		{"extra exact host", "postgres://u:p@db.vpce.internal:5432/db", Pooled, []Option{WithHostSuffixes("db.vpce.internal")}, PoolerPort},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			port, _, _ := mustConfig(t, c.dsn, c.mode, c.opts...)
			if port != c.port {
				t.Fatalf("port = %d, want %d", port, c.port)
			}
		})
	}
}

func TestRoutingKeepsCredentialsAndHost(t *testing.T) {
	cfg, err := Config(psDSN, Pooled, noEnv())
	if err != nil {
		t.Fatal(err)
	}
	cc := cfg.ConnConfig
	if cc.Host != "abc123-apsoutheast2-1.horizon.psdb.cloud" || cc.User != "app.abc123" || cc.Password != "p w'x" || cc.Database != "drover" {
		t.Fatalf("connection fields changed: host=%q user=%q db=%q", cc.Host, cc.User, cc.Database)
	}
	if cc.TLSConfig == nil || cc.TLSConfig.ServerName != "" && cc.TLSConfig.ServerName != cc.Host {
		t.Fatalf("TLS config unexpected: %+v", cc.TLSConfig)
	}
}

func TestFallbacks(t *testing.T) {
	dsn := "postgres://u:p@a-useast1-1.horizon.psdb.cloud:5432,localhost:5432/db?sslmode=disable"
	cfg, err := Config(dsn, Pooled, noEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Port != PoolerPort {
		t.Fatalf("primary port = %d", cfg.ConnConfig.Port)
	}
	if len(cfg.ConnConfig.Fallbacks) != 1 || cfg.ConnConfig.Fallbacks[0].Port != DirectPort {
		t.Fatalf("fallbacks = %+v", cfg.ConnConfig.Fallbacks)
	}

	cfg, err = Config("postgres://u:p@localhost:5432,b-useast1-1.horizon.psdb.cloud:5432/db?sslmode=disable", Pooled, noEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Fallbacks[0].Port != PoolerPort {
		t.Fatalf("fallback port = %d", cfg.ConnConfig.Fallbacks[0].Port)
	}
	if cfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeCacheDescribe {
		t.Fatalf("exec mode = %v, want cache_describe when a fallback is on the pooler", cfg.ConnConfig.DefaultQueryExecMode)
	}
}

func TestMaxConns(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		mode Mode
		opts []Option
		want int32
	}{
		{"pooled default", psURL, Pooled, nil, DefaultPooledMaxConns},
		{"pooled default local", localDB, Pooled, nil, DefaultPooledMaxConns},
		{"session default", psURL, Session, nil, DefaultSessionMaxConns},
		{"url pool_max_conns", psURL + "&pool_max_conns=10", Pooled, nil, 10},
		{"dsn pool_max_conns", psDSN + " pool_max_conns=7", Session, nil, 7},
		{"option beats url", psURL + "&pool_max_conns=10", Pooled, []Option{WithMaxConns(5)}, 5},
		{"option zero ignored", psURL, Pooled, []Option{WithMaxConns(0)}, DefaultPooledMaxConns},
		{"pooled default clamped high", psURL, Pooled, []Option{WithDefaultMaxConns(50)}, 4},
		{"pooled default clamped low", psURL, Pooled, []Option{WithDefaultMaxConns(1)}, 2},
		{"session default clamped low", psURL, Session, []Option{WithDefaultMaxConns(-3)}, 1},
		{"session default in range", psURL, Session, []Option{WithDefaultMaxConns(3)}, 3},
		{"url beats default option", psURL + "&pool_max_conns=9", Pooled, []Option{WithDefaultMaxConns(2)}, 9},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got, _ := mustConfig(t, c.dsn, c.mode, c.opts...)
			if got != c.want {
				t.Fatalf("MaxConns = %d, want %d", got, c.want)
			}
		})
	}
}

func TestMinConnsNotAboveMax(t *testing.T) {
	cfg, err := Config(psURL+"&pool_min_conns=8", Pooled, noEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinConns != cfg.MaxConns {
		t.Fatalf("MinConns = %d, MaxConns = %d", cfg.MinConns, cfg.MaxConns)
	}
}

func TestExecMode(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
		mode Mode
		opts []Option
		want pgx.QueryExecMode
	}{
		{"pooled on pooler", psURL, Pooled, nil, pgx.QueryExecModeCacheDescribe},
		{"pooled local keeps default", localDB, Pooled, nil, pgx.QueryExecModeCacheStatement},
		{"session keeps default", psURL, Session, nil, pgx.QueryExecModeCacheStatement},
		{"url exec mode respected", psURL + "&default_query_exec_mode=exec", Pooled, nil, pgx.QueryExecModeExec},
		{"option wins", psURL, Pooled, []Option{WithExecMode(pgx.QueryExecModeCacheStatement)}, pgx.QueryExecModeCacheStatement},
		{"option on session", psURL, Session, []Option{WithExecMode(pgx.QueryExecModeExec)}, pgx.QueryExecModeExec},
		{"generic pgbouncer 6432", "postgres://u:p@pgbouncer:6432/db?sslmode=disable", Pooled, nil, pgx.QueryExecModeCacheDescribe},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, got := mustConfig(t, c.dsn, c.mode, c.opts...)
			if got != c.want {
				t.Fatalf("exec mode = %v, want %v", got, c.want)
			}
		})
	}
}

func TestEnvPolicy(t *testing.T) {
	cases := []struct {
		env  string
		dsn  string
		port uint16
	}{
		{"off", psURL, DirectPort},
		{" OFF ", psURL, DirectPort},
		{"force", localDB, PoolerPort},
		{"auto", psURL, PoolerPort},
		{"", psURL, PoolerPort},
	}
	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			cfg, err := Config(c.dsn, Pooled, env(c.env))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ConnConfig.Port != c.port {
				t.Fatalf("port = %d, want %d", cfg.ConnConfig.Port, c.port)
			}
		})
	}
	cfg, err := Config(psURL, Pooled, env("off"), WithPooler(PolicyAuto))
	if err != nil || cfg.ConnConfig.Port != PoolerPort {
		t.Fatalf("WithPooler should override env: port=%v err=%v", cfg, err)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Config(psURL, Pooled, env("sometimes")); err == nil || !strings.Contains(err.Error(), "sometimes") {
		t.Fatalf("invalid env policy: err = %v", err)
	}
	if _, err := Config(psURL, Pooled, noEnv(), WithPooler("maybe")); err == nil {
		t.Fatal("invalid option policy accepted")
	}
	if _, err := Config(psURL, Mode(0), noEnv()); err == nil {
		t.Fatal("zero mode accepted")
	}
	if _, err := Config("postgres://u:p@host:notaport/db", Pooled, noEnv()); err == nil {
		t.Fatal("bad connection string accepted")
	}
	if _, err := New(context.Background(), psURL, Mode(9)); err == nil {
		t.Fatal("New accepted bad mode")
	}
}

func TestNewDoesNotConnect(t *testing.T) {
	p, err := New(context.Background(), "postgres://u:p@127.0.0.1:1/db?sslmode=disable", Session, noEnv())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if got := p.Config().MaxConns; got != DefaultSessionMaxConns {
		t.Fatalf("MaxConns = %d", got)
	}
}

func TestIsPlanetScaleHost(t *testing.T) {
	for host, want := range map[string]bool{
		"abc-useast1-1.horizon.psdb.cloud": true,
		"horizon.psdb.cloud":               true,
		"aws.connect.psdb.cloud":           false,
		"localhost":                        false,
		"xhorizon.psdb.cloud":              false,
		"":                                 false,
	} {
		if got := IsPlanetScaleHost(host); got != want {
			t.Errorf("IsPlanetScaleHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestModeString(t *testing.T) {
	if Pooled.String() != "pooled" || Session.String() != "session" || Mode(7).String() != "Mode(7)" {
		t.Fatal("Mode.String")
	}
}

func TestConnParams(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		not  []string
	}{
		{"postgres://u@h/db?pool_max_conns=3&sslmode=disable", []string{"pool_max_conns", "sslmode"}, []string{"u"}},
		{"postgresql://u@h/db", nil, []string{"pool_max_conns"}},
		{"postgres://%zz", nil, []string{"pool_max_conns"}},
		{"host=h password='a pool_max_conns=9 \\' b' dbname=x", []string{"host", "password", "dbname"}, []string{"pool_max_conns"}},
		{"  host = h  pool_max_conns=2", []string{"host", "pool_max_conns"}, nil},
		{"host=h password='unterminated", []string{"host", "password"}, nil},
		{"noequals", nil, []string{"noequals"}},
	}
	for _, c := range cases {
		got := connParams(c.in)
		for _, k := range c.want {
			if !got[k] {
				t.Errorf("connParams(%q) missing %q: %v", c.in, k, got)
			}
		}
		for _, k := range c.not {
			if got[k] {
				t.Errorf("connParams(%q) has %q: %v", c.in, k, got)
			}
		}
	}
}
