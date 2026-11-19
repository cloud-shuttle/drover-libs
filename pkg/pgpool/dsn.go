package pgpool

import (
	"net/url"
	"strings"
)

// connParams returns the parameter names set in a connection string, so
// explicit settings such as pool_max_conns can be told apart from pgx
// defaults after ParseConfig has consumed them.
func connParams(s string) map[string]bool {
	if strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://") {
		return urlParams(s)
	}
	return keywordParams(s)
}

func urlParams(s string) map[string]bool {
	set := map[string]bool{}
	u, err := url.Parse(s)
	if err != nil {
		return set
	}
	for k := range u.Query() {
		set[k] = true
	}
	return set
}

// keywordParams scans a libpq key=value DSN. Values may be single-quoted
// with backslash escapes. ParseConfig has already validated the string.
func keywordParams(s string) map[string]bool {
	set := map[string]bool{}
	for s = strings.TrimLeft(s, " \t\n\r"); s != ""; s = strings.TrimLeft(s, " \t\n\r") {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		set[strings.TrimSpace(s[:eq])] = true
		s = skipValue(strings.TrimLeft(s[eq+1:], " \t\n\r"))
	}
	return set
}

// skipValue returns s after its leading value.
func skipValue(s string) string {
	if !strings.HasPrefix(s, "'") {
		if i := strings.IndexAny(s, " \t\n\r"); i >= 0 {
			return s[i:]
		}
		return ""
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '\'':
			return s[i+1:]
		}
	}
	return ""
}
