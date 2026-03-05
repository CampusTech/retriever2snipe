package retriever

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToMax(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("Wait() call %d: %v", i, err)
		}
	}
	// All 3 tokens consumed
	if rl.tokens != 0 {
		t.Errorf("tokens = %d, want 0", rl.tokens)
	}
}

func TestRateLimiterBlocksWhenEmpty(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	ctx := context.Background()

	// Consume the one token
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("first Wait: %v", err)
	}

	// Next call should block; cancel after short timeout
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Fatal("expected context error when no tokens available")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestRateLimiterRefills(t *testing.T) {
	rl := newRateLimiter(2, 50*time.Millisecond) // very short window for testing
	ctx := context.Background()

	// Consume both tokens
	rl.Wait(ctx)
	rl.Wait(ctx)

	// Wait for refill
	time.Sleep(60 * time.Millisecond)

	// Should succeed now
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait after refill: %v", err)
	}
}

func TestRateLimiterContextCancellation(t *testing.T) {
	rl := newRateLimiter(0, time.Hour) // zero tokens, long refill = always blocks
	// Force tokens to 0
	rl.tokens = 0

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := rl.Wait(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if err != context.Canceled {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestNewRateLimiter(t *testing.T) {
	rl := newRateLimiter(50, time.Minute)

	if rl.tokens != 50 {
		t.Errorf("tokens = %d, want 50", rl.tokens)
	}
	if rl.maxTokens != 50 {
		t.Errorf("maxTokens = %d, want 50", rl.maxTokens)
	}
	if rl.refillRate != time.Minute {
		t.Errorf("refillRate = %v, want %v", rl.refillRate, time.Minute)
	}
}
