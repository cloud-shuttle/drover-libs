package clock

import (
	"sync"
	"testing"
	"time"
)

var (
	_ Clock = RealClock{}
	_ Clock = (*MockClock)(nil)
)

func TestRealClockTracksWallTime(t *testing.T) {
	c := RealClock{}
	before := time.Now()
	got := c.Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("Now() = %v, want between %v and %v", got, before, after)
	}
	if d := c.Since(before); d < 0 {
		t.Fatalf("Since(before) = %v, want >= 0", d)
	}
}

func TestMockClockIsFrozenUntilMoved(t *testing.T) {
	start := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	m := NewMockClock(start)

	if got := m.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
	time.Sleep(time.Millisecond)
	if got := m.Now(); !got.Equal(start) {
		t.Fatalf("Now() moved without Add: %v", got)
	}

	m.Add(90 * time.Second)
	if got, want := m.Now(), start.Add(90*time.Second); !got.Equal(want) {
		t.Fatalf("after Add: Now() = %v, want %v", got, want)
	}
	if got := m.Since(start); got != 90*time.Second {
		t.Fatalf("Since(start) = %v, want 90s", got)
	}

	earlier := start.Add(-time.Hour)
	m.SetTime(earlier)
	if got := m.Now(); !got.Equal(earlier) {
		t.Fatalf("after SetTime: Now() = %v, want %v", got, earlier)
	}
	if got := m.Since(start); got != -time.Hour {
		t.Fatalf("Since(start) after rewind = %v, want -1h", got)
	}
}

func TestMockClockConcurrentUse(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewMockClock(start)

	const workers, steps = 8, 100
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range steps {
				m.Add(time.Millisecond)
				_ = m.Now()
				_ = m.Since(start)
			}
		}()
	}
	wg.Wait()

	if got, want := m.Since(start), workers*steps*time.Millisecond; got != want {
		t.Fatalf("Since(start) = %v, want %v", got, want)
	}
}
