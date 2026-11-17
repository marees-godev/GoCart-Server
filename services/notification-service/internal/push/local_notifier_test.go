package push

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestLocalSystemNotifier_Disabled(t *testing.T) {
	notifier := NewLocalSystemNotifier(slog.Default(), false)
	if notifier.IsEnabled() {
		t.Fatal("expected notifier to be disabled")
	}

	ctx := context.Background()
	err := notifier.Notify(ctx, "Your order #12345 has been confirmed!")
	if err != nil {
		t.Fatalf("expected nil error when disabled, got: %v", err)
	}

	// NotifyAsync should also not panic
	notifier.NotifyAsync("Your order #12345 has been confirmed!")
}

func TestLocalSystemNotifier_EnabledContentOnly(t *testing.T) {
	notifier := NewLocalSystemNotifier(nil, true)
	if !notifier.IsEnabled() {
		t.Fatal("expected notifier to be enabled")
	}

	// Asynchronous dispatch should execute with only content
	notifier.NotifyAsync("Order #12345 has been delivered to your doorstep.")
	time.Sleep(100 * time.Millisecond)
}
