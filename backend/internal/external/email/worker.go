package email

import (
	"context"
	"log"
	"time"
)

const (
	maxAttempts = 4
)

// backoffDurations defines the wait time before each retry attempt.
// Index 0 is the delay before the 2nd attempt, index 1 before the 3rd, etc.
var backoffDurations = [maxAttempts - 1]time.Duration{
	500 * time.Millisecond,
	2 * time.Second,
	8 * time.Second,
}

// SendWithRetry delivers an email with exponential backoff on transient errors.
// It is designed to run in a goroutine — the caller should not wait for it.
// A dedicated context with a long timeout is derived from ctx so that HTTP
// request cancellation does not abort in-flight delivery attempts.
func SendWithRetry(ctx context.Context, sender Sender, to, subject, body string) {
	// Cap total time to slightly beyond the longest possible retry sequence.
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if timeoutCtx.Err() != nil {
			log.Printf("email: delivery to %s aborted after %d attempt(s): context expired", to, attempt-1)
			return
		}

		lastErr = sender.Send(to, subject, body)
		if lastErr == nil {
			return // delivered
		}

		if !IsTransient(lastErr) {
			log.Printf("email: permanent failure delivering to %s: %v", to, lastErr)
			return
		}

		if attempt < maxAttempts {
			delay := backoffDurations[attempt-1]
			log.Printf("email: transient failure delivering to %s (attempt %d/%d), retrying in %s: %v",
				to, attempt, maxAttempts, delay, lastErr)

			select {
			case <-timeoutCtx.Done():
				log.Printf("email: delivery to %s aborted while waiting to retry: context expired", to)
				return
			case <-time.After(delay):
			}
		}
	}

	log.Printf("email: all %d attempts failed delivering to %s: %v", maxAttempts, to, lastErr)
}
