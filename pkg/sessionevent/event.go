// Package sessionevent is the shared session event schema (org ADR-003,
// version 1): one JSON object per line, written by the orchestrator
// (drover), the coding worker (drover-code), the dvr host and, from M2,
// Kelpies sessions.
//
// It replaces the two hand-synchronised copies in drover/internal/sessionevent
// and drover-code/pkg/sessionevent. The wire format, constants and the New /
// Append / ReadAll functions are unchanged, so moving a caller over is an
// import path change. Additions are optional fields (trace_id, span_id,
// turn) that older readers ignore, and stream helpers (Encode, Decode) for
// writers that are not files, such as a DBOS stream or an SSE response.
package sessionevent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Version is the schema version written in every event's "v" field.
const Version = 1

// Event types in schema version 1.
const (
	TypeStarted           = "session.started"
	TypeStatus            = "session.status"
	TypeMessage           = "message.appended"
	TypeToolRequested     = "tool.requested"
	TypeToolFinished      = "tool.finished"
	TypeApprovalRequested = "approval.requested"
	TypeApprovalResolved  = "approval.resolved"
	TypeArtifactWritten   = "artifact.written"
	TypeLinked            = "session.linked"
)

var knownTypes = map[string]bool{
	TypeStarted: true, TypeStatus: true, TypeMessage: true,
	TypeToolRequested: true, TypeToolFinished: true,
	TypeApprovalRequested: true, TypeApprovalResolved: true,
	TypeArtifactWritten: true, TypeLinked: true,
}

// KnownType reports whether typ is a version 1 event type.
func KnownType(typ string) bool { return knownTypes[typ] }

// Event is one session-event line. Field names are the contract; do not
// rename them.
type Event struct {
	V         int            `json:"v"`
	Type      string         `json:"type"`
	TS        string         `json:"ts"`
	SessionID string         `json:"session_id"`
	IssueID   string         `json:"issue_id,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Data      map[string]any `json:"data,omitempty"`

	// Turn is the 1-based turn number within a multi-turn session. Zero
	// means the event is not part of a session turn and is omitted.
	Turn int `json:"turn,omitempty"`
	// TraceID and SpanID are the W3C trace context (lowercase hex) of the
	// span that emitted the event, so the run viewer can join the event log
	// to otel_traces. drover-libs/pkg/telemetry TraceID and SpanID return them.
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}

// New builds an event stamped at the current UTC time.
func New(typ, sessionID string, data map[string]any) Event {
	return Event{
		V:         Version,
		Type:      typ,
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		SessionID: sessionID,
		Data:      data,
	}
}

// Validate reports the first problem that would make ev unreadable by
// another writer's tools: wrong version, unknown type, missing session ID or
// a timestamp that is not RFC 3339.
func (ev Event) Validate() error {
	if ev.V != Version {
		return fmt.Errorf("sessionevent: v=%d, want %d", ev.V, Version)
	}
	if !KnownType(ev.Type) {
		return fmt.Errorf("sessionevent: unknown type %q", ev.Type)
	}
	if ev.SessionID == "" {
		return errors.New("sessionevent: session_id is empty")
	}
	if _, err := time.Parse(time.RFC3339Nano, ev.TS); err != nil {
		return fmt.Errorf("sessionevent: ts %q is not RFC 3339: %w", ev.TS, err)
	}
	if ev.Turn < 0 {
		return fmt.Errorf("sessionevent: turn %d is negative", ev.Turn)
	}
	return nil
}

func stamp(ev Event) Event {
	if ev.V == 0 {
		ev.V = Version
	}
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return ev
}

// Encode writes ev to w as one JSON line, filling V and TS if unset.
func Encode(w io.Writer, ev Event) error {
	return json.NewEncoder(w).Encode(stamp(ev))
}

// Marshal returns ev as one JSON line (with trailing newline), filling V and
// TS if unset.
func Marshal(ev Event) ([]byte, error) {
	var buf bytes.Buffer
	if err := Encode(&buf, ev); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode reads every event from r until EOF.
func Decode(r io.Reader) ([]Event, error) {
	var out []Event
	dec := json.NewDecoder(bufio.NewReader(r))
	for dec.More() {
		var ev Event
		if err := dec.Decode(&ev); err != nil {
			return out, fmt.Errorf("sessionevent: event %d: %w", len(out)+1, err)
		}
		out = append(out, ev)
	}
	return out, nil
}

var fileMu sync.Map // path -> *sync.Mutex

// Append writes one event as a single JSON line to the file at path,
// creating parent directories. Concurrent Appends to the same path from one
// process are serialised.
func Append(path string, ev Event) error {
	mu := mutexFor(path)
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if err := Encode(f, ev); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ReadAll parses every line of the file at path.
func ReadAll(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

func mutexFor(path string) *sync.Mutex {
	v, _ := fileMu.LoadOrStore(path, &sync.Mutex{})
	return v.(*sync.Mutex)
}
