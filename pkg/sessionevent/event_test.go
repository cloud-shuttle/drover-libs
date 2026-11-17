package sessionevent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testdata/v1-drover-code.jsonl was written by drover-code/pkg/sessionevent
// (commit 9c069be) with Append. Writing the same events here must produce
// the same bytes, so callers can switch packages without changing files on
// disk or the readers that parse them.
func goldenEvents() []Event {
	return []Event{
		{V: 1, Type: TypeStarted, TS: "2026-11-18T01:02:03.000000004Z", SessionID: "s-1", IssueID: "ISS-9", Phase: "plan", Data: map[string]any{"b": 2, "a": "x"}},
		{V: 1, Type: TypeToolFinished, TS: "2026-11-18T01:02:04Z", SessionID: "s-1", Data: map[string]any{"tool": "git.push", "ok": true}},
		{V: 1, Type: TypeStatus, TS: "2026-11-18T01:02:05Z", SessionID: "s-1"},
	}
}

func TestWireFormatMatchesDroverCode(t *testing.T) {
	want, err := os.ReadFile("testdata/v1-drover-code.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "nested", "session.jsonl")
	for _, ev := range goldenEvents() {
		if err := Append(p, ev); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("wire format changed\n got: %s\nwant: %s", got, want)
	}

	evs, err := ReadAll("testdata/v1-drover-code.jsonl")
	if err != nil {
		t.Fatalf("ReadAll golden: %v", err)
	}
	if len(evs) != 3 || evs[0].IssueID != "ISS-9" || evs[1].Data["tool"] != "git.push" {
		t.Fatalf("decoded golden = %+v", evs)
	}
	for i, ev := range evs {
		if err := ev.Validate(); err != nil {
			t.Errorf("golden event %d: %v", i, err)
		}
	}
}

func TestOptionalFieldsAreOmittedAndRoundTrip(t *testing.T) {
	ev := New(TypeMessage, "s-2", map[string]any{"role": "user"})
	line, err := Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{`"turn"`, `"trace_id"`, `"span_id"`, `"issue_id"`, `"phase"`} {
		if bytes.Contains(line, []byte(k)) {
			t.Errorf("unset %s should be omitted: %s", k, line)
		}
	}

	ev.Turn = 3
	ev.TraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	ev.SpanID = "00f067aa0ba902b7"
	var buf bytes.Buffer
	if err := Encode(&buf, ev); err != nil {
		t.Fatal(err)
	}
	if err := Encode(&buf, New(TypeStatus, "s-2", nil)); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Turn != 3 || got[0].TraceID != ev.TraceID || got[0].SpanID != ev.SpanID {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestEncodeFillsVersionAndTimestamp(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	line, err := Marshal(Event{Type: TypeLinked, SessionID: "s-3"})
	if err != nil {
		t.Fatal(err)
	}
	evs, err := Decode(bytes.NewReader(line))
	if err != nil || len(evs) != 1 {
		t.Fatalf("Decode: %v %v", evs, err)
	}
	ev := evs[0]
	if ev.V != Version {
		t.Errorf("v = %d, want %d", ev.V, Version)
	}
	ts, err := time.Parse(time.RFC3339Nano, ev.TS)
	if err != nil || ts.Before(before) {
		t.Errorf("ts = %q (%v), want a current RFC 3339 time", ev.TS, err)
	}
	if !strings.HasSuffix(string(line), "\n") {
		t.Errorf("Marshal should end the line with a newline")
	}
}

func TestValidate(t *testing.T) {
	ok := New(TypeStarted, "s", nil)
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid event: %v", err)
	}
	bad := map[string]Event{
		"version":       {V: 2, Type: TypeStarted, TS: ok.TS, SessionID: "s"},
		"type":          {V: 1, Type: "session.exploded", TS: ok.TS, SessionID: "s"},
		"session":       {V: 1, Type: TypeStarted, TS: ok.TS},
		"timestamp":     {V: 1, Type: TypeStarted, TS: "yesterday", SessionID: "s"},
		"negative turn": {V: 1, Type: TypeStarted, TS: ok.TS, SessionID: "s", Turn: -1},
	}
	for name, ev := range bad {
		if err := ev.Validate(); err == nil {
			t.Errorf("%s: Validate accepted %+v", name, ev)
		}
	}
	for _, typ := range []string{TypeStarted, TypeStatus, TypeMessage, TypeToolRequested, TypeToolFinished,
		TypeApprovalRequested, TypeApprovalResolved, TypeArtifactWritten, TypeLinked} {
		if !KnownType(typ) {
			t.Errorf("KnownType(%q) = false", typ)
		}
	}
}

func TestDecodeReportsBadLine(t *testing.T) {
	in := `{"v":1,"type":"session.status","ts":"2026-11-18T00:00:00Z","session_id":"s"}` + "\n{not json\n"
	evs, err := Decode(strings.NewReader(in))
	if err == nil || !strings.Contains(err.Error(), "event 2") {
		t.Fatalf("Decode err = %v, want it to name event 2", err)
	}
	if len(evs) != 1 {
		t.Fatalf("Decode should return the events before the bad line, got %d", len(evs))
	}
	if _, err := ReadAll(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil {
		t.Fatalf("ReadAll of a missing file should fail")
	}
}

func TestConcurrentAppendKeepsLinesWhole(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	const writers, each = 8, 50
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				ev := New(TypeMessage, "s", map[string]any{"text": strings.Repeat(fmt.Sprint(w), 200), "i": i})
				if err := Append(p, ev); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	evs, err := ReadAll(p)
	if err != nil {
		t.Fatalf("ReadAll after concurrent appends: %v", err)
	}
	if len(evs) != writers*each {
		t.Fatalf("got %d events, want %d", len(evs), writers*each)
	}
}

func TestAppendFailsOnUnwritablePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a-file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Append(filepath.Join(file, "child.jsonl"), New(TypeStatus, "s", nil)); err == nil {
		t.Fatalf("Append under a regular file should fail")
	}
	if err := Append(dir, New(TypeStatus, "s", nil)); err == nil {
		t.Fatalf("Append to a directory path should fail")
	}
	if _, err := Marshal(Event{Type: TypeStatus, SessionID: "s", Data: map[string]any{"bad": make(chan int)}}); err == nil {
		t.Fatalf("Marshal of an unencodable value should fail")
	}
}
