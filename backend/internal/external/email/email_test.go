package email

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- helpers ---

// mockNetError implements net.Error so we can test the IsTransient net-error path.
type mockNetError struct{ msg string }

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return true }
func (e *mockNetError) Temporary() bool { return true }

// mockSender records every Send call and returns errors from a queue.
type mockSender struct {
	calls  atomic.Int32
	errors []error // errors[i] is returned on the (i+1)th call; nil means success
}

func (m *mockSender) Send(_, _, _ string) error {
	i := int(m.calls.Add(1)) - 1
	if i < len(m.errors) {
		return m.errors[i]
	}
	return nil
}

// zeroBackoff replaces backoffDurations with zeros for fast tests, and restores
// on cleanup.
func zeroBackoff(t *testing.T) {
	t.Helper()
	orig := backoffDurations
	backoffDurations = [maxAttempts - 1]time.Duration{}
	t.Cleanup(func() { backoffDurations = orig })
}

// --- buildMessage ---

func TestBuildMessage_ContainsRequiredHeaders(t *testing.T) {
	msg := buildMessage("from@example.com", "to@example.com", "Hello", "body text")

	checks := []string{
		"From: from@example.com\r\n",
		"To: to@example.com\r\n",
		"Subject: Hello\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
	}
	for _, want := range checks {
		if !strings.Contains(msg, want) {
			t.Errorf("expected header %q in message", want)
		}
	}
}

func TestBuildMessage_HeadersSeparatedFromBodyByBlankLine(t *testing.T) {
	msg := buildMessage("a@b.com", "c@d.com", "subj", "hello world")
	// RFC 2822: blank line (\r\n\r\n) separates headers from body
	if !strings.Contains(msg, "\r\n\r\n") {
		t.Error("expected blank line between headers and body")
	}
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 || parts[1] != "hello world" {
		t.Errorf("expected body after blank line, got %q", parts[1])
	}
}

// --- IsTransient ---

func TestIsTransient_Nil(t *testing.T) {
	if IsTransient(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsTransient_SMTP4xx(t *testing.T) {
	for _, code := range []string{"450", "451", "452"} {
		err := errors.New(code + " try again later")
		if !IsTransient(err) {
			t.Errorf("expected SMTP %s to be transient", code)
		}
	}
}

func TestIsTransient_SMTP5xx_IsPermanent(t *testing.T) {
	for _, code := range []string{"500", "550", "553"} {
		err := errors.New(code + " permanent failure")
		if IsTransient(err) {
			t.Errorf("expected SMTP %s to be permanent (not transient)", code)
		}
	}
}

func TestIsTransient_NetError(t *testing.T) {
	if !IsTransient(&mockNetError{msg: "connection reset"}) {
		t.Error("expected net.Error to be transient")
	}
}

func TestIsTransient_PlainError_IsPermanent(t *testing.T) {
	if IsTransient(errors.New("some unexpected error")) {
		t.Error("expected plain error to be permanent")
	}
}

// --- SendWithRetry ---

func TestSendWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	zeroBackoff(t)
	s := &mockSender{}
	SendWithRetry(context.Background(), s, "to@example.com", "subj", "body")
	if got := s.calls.Load(); got != 1 {
		t.Errorf("expected 1 Send call, got %d", got)
	}
}

func TestSendWithRetry_SuccessAfterTransientFailure(t *testing.T) {
	zeroBackoff(t)
	transient := errors.New("451 try again")
	s := &mockSender{errors: []error{transient, nil}}
	SendWithRetry(context.Background(), s, "to@example.com", "subj", "body")
	if got := s.calls.Load(); got != 2 {
		t.Errorf("expected 2 Send calls, got %d", got)
	}
}

func TestSendWithRetry_PermanentFailureStopsAfterOneAttempt(t *testing.T) {
	zeroBackoff(t)
	permanent := errors.New("550 user not found")
	s := &mockSender{errors: []error{permanent}}
	SendWithRetry(context.Background(), s, "to@example.com", "subj", "body")
	if got := s.calls.Load(); got != 1 {
		t.Errorf("expected 1 Send call on permanent error, got %d", got)
	}
}

func TestSendWithRetry_ExhaustsAllAttempts(t *testing.T) {
	zeroBackoff(t)
	transient := errors.New("452 insufficient storage")
	s := &mockSender{errors: []error{transient, transient, transient, transient}}
	SendWithRetry(context.Background(), s, "to@example.com", "subj", "body")
	if got := s.calls.Load(); got != maxAttempts {
		t.Errorf("expected %d Send calls, got %d", maxAttempts, got)
	}
}

func TestSendWithRetry_AlreadyCancelledContext_DoesNotSend(t *testing.T) {
	zeroBackoff(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	s := &mockSender{}
	SendWithRetry(ctx, s, "to@example.com", "subj", "body")
	// The context check happens at the top of the loop, before the first Send.
	if got := s.calls.Load(); got != 0 {
		t.Errorf("expected 0 Send calls with pre-cancelled context, got %d", got)
	}
}

func TestSendWithRetry_ContextCancelledDuringBackoff(t *testing.T) {
	// Use a real (short) delay so the select can actually fire.
	orig := backoffDurations
	backoffDurations = [maxAttempts - 1]time.Duration{
		50 * time.Millisecond,
		50 * time.Millisecond,
		50 * time.Millisecond,
	}
	t.Cleanup(func() { backoffDurations = orig })

	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	s := &mockSender{}
	// Replace errors so we always get transient, then cancel the context mid-retry.
	s.errors = make([]error, maxAttempts)
	for i := range s.errors {
		s.errors[i] = errors.New("451 slow down")
	}

	// Cancel after the first Send so the wait is interrupted.
	originalSend := s.errors
	_ = originalSend
	cancelSender := &callbackSender{
		fn: func(attempt int32) error {
			if attempt == 1 {
				go func() {
					time.Sleep(10 * time.Millisecond)
					cancel()
				}()
			}
			calls.Add(1)
			return errors.New("451 slow down")
		},
	}

	SendWithRetry(ctx, cancelSender, "to@example.com", "subj", "body")

	// Should have sent exactly once before the context was cancelled during the backoff wait.
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 Send call before context cancel, got %d", got)
	}
}

// callbackSender is a Sender whose Send invokes a function with the attempt number.
type callbackSender struct {
	attempt atomic.Int32
	fn      func(attempt int32) error
}

func (c *callbackSender) Send(_, _, _ string) error {
	return c.fn(c.attempt.Add(1))
}
