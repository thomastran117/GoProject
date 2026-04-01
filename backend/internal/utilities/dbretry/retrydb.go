package dbretry

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"backend/internal/utilities/logger"
)

const retryMax = 3
const retryBase = 100 * time.Millisecond
const retryMaxDelay = 1 * time.Second

// Do executes op with exponential backoff retry for transient DB errors.
// Non-transient errors (unique constraint violations, record not found, etc.)
// are returned immediately without retrying.
func Do(op func() error) error {
	var lastErr error

	for attempt := 0; attempt < retryMax; attempt++ {
		if attempt > 0 {
			delay := retryBase << uint(attempt-1)
			if delay > retryMaxDelay {
				delay = retryMaxDelay
			}
			jitter := time.Duration(rand.Int64N(int64(delay / 2)))
			time.Sleep(delay + jitter)
			logger.Warn("db: retrying operation (attempt %d/%d) after error: %v", attempt+1, retryMax, lastErr)
		}

		err := op()
		if err == nil {
			return nil
		}
		if !isTransient(err) {
			return err
		}
		lastErr = err
	}

	return fmt.Errorf("db: operation failed after %d attempts: %w", retryMax, lastErr)
}

// isTransient reports whether err is a transient DB error that is safe to retry.
// Deadlocks (1213) and lock wait timeouts (1205) are transient. Unique constraint
// violations (1062) and all other MySQL errors are permanent. Non-MySQL errors
// (network drops, driver-level connection failures) are treated as transient.
func isTransient(err error) bool {
	if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1205, // ER_LOCK_WAIT_TIMEOUT
			1213: // ER_LOCK_DEADLOCK
			return true
		}
		return false
	}
	// Non-MySQL error — treat as transient (network/connection loss).
	return true
}
