package circuitbreaker

import (
	"testing"
	"time"
)

func TestAllowInitially(t *testing.T) {
	cb := New(3, time.Minute)
	if !cb.Allow() {
		t.Error("expected Allow to return true initially")
	}
}

func TestOpensAfterThreshold(t *testing.T) {
	cb := New(3, time.Minute)
	cb.Failure()
	cb.Failure()
	cb.Failure()
	if cb.Allow() {
		t.Error("expected Allow to return false after threshold failures")
	}
}

func TestResetsAfterSuccess(t *testing.T) {
	cb := New(3, time.Minute)
	cb.Failure()
	cb.Success()
	if !cb.Allow() {
		t.Error("expected Allow to return true after success resets")
	}
}

func TestStaysClosedBelowThreshold(t *testing.T) {
	cb := New(3, time.Minute)
	cb.Failure()
	cb.Failure()
	if !cb.Allow() {
		t.Error("expected Allow to return true before reaching threshold")
	}
}

func TestHalfOpenAfterDuration(t *testing.T) {
	cb := New(1, 50*time.Millisecond)
	cb.Failure()
	if cb.Allow() {
		t.Error("expected Allow to return false immediately after failure")
	}
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Error("expected Allow to return true after reset duration")
	}
}
