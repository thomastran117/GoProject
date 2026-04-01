package device

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"backend/internal/utilities/dbretry"
)

// Device represents a known device for a user, identified by a SHA-256 hash of
// the sanitized User-Agent string. The composite unique index on (user_id,
// fingerprint) ensures each device is recorded once per user.
type Device struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	UserID      uint64    `gorm:"not null;uniqueIndex:idx_user_fingerprint"`
	Fingerprint string    `gorm:"not null;uniqueIndex:idx_user_fingerprint;size:64"`
	DeviceType  string    `gorm:"not null;size:20"`
	UserAgent   string    `gorm:"size:512"`
	LastSeenAt  time.Time
	CreatedAt   time.Time
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Fingerprint computes the SHA-256 hex digest of the given User-Agent string.
// The middleware already sanitizes and caps the UA at 512 chars, so this input
// is stable and bounded.
//
// Limitation: User-Agent is the only signal used here. It is easy to spoof and
// shared across devices with the same browser/OS combination, which can produce
// false positives (treating a different device as known) or false negatives
// (treating a reinstalled browser as a new device). This is intentional as a
// low-friction baseline; callers that need stronger guarantees should layer
// additional signals (e.g. a stable client-generated device ID sent in a
// custom header) into the fingerprint before hashing.
func Fingerprint(userAgent string) string {
	sum := sha256.Sum256([]byte(userAgent))
	return hex.EncodeToString(sum[:])
}

// FindByUserAndFingerprint returns the device record for the given user and
// fingerprint, or nil if no such device exists.
func (r *Repository) FindByUserAndFingerprint(userID uint64, fingerprint string) (*Device, error) {
	var d Device
	err := dbretry.Do(func() error {
		return r.db.Where("user_id = ? AND fingerprint = ?", userID, fingerprint).First(&d).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Create inserts a new device record for the user.
func (r *Repository) Create(userID uint64, fingerprint, deviceType, userAgent string) (*Device, error) {
	now := time.Now()
	d := &Device{
		UserID:      userID,
		Fingerprint: fingerprint,
		DeviceType:  deviceType,
		UserAgent:   userAgent,
		LastSeenAt:  now,
	}
	err := dbretry.Do(func() error {
		return r.db.Create(d).Error
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// UpdateLastSeen sets LastSeenAt to now for the given device ID.
func (r *Repository) UpdateLastSeen(id uint64) error {
	return dbretry.Do(func() error {
		return r.db.Model(&Device{}).Where("id = ?", id).Update("last_seen_at", time.Now()).Error
	})
}
