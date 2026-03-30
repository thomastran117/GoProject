package school

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/application/middleware"
)

// School is the database model for a school entity. Each school is owned by a
// principal (PrincipalID) and carries identifying metadata.
type School struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	PrincipalID uint64    `gorm:"index;not null"`
	Name        string    `gorm:"uniqueIndex;size:200;not null"`
	Address     string    `gorm:"size:500"`
	City        string    `gorm:"size:100;not null"`
	Country     string    `gorm:"size:100;not null"`
	Phone       string    `gorm:"size:30"`
	Email       string    `gorm:"size:254"`
	Website     string    `gorm:"size:2048"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Repository wraps the GORM database connection and provides all persistence
// operations for the School model.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a Repository backed by the given GORM instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// FindByID returns the school with the given primary key, or nil if no row
// exists. Any unexpected database error is returned as a non-nil error.
func (r *Repository) FindByID(id uint64) (*School, error) {
	var s School
	result := r.db.First(&s, id)
	if result.Error == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &s, nil
}

// FindAll returns every school row in the table. Returns an empty slice (not
// nil) when no schools exist.
func (r *Repository) FindAll() ([]*School, error) {
	var schools []*School
	if err := r.db.Find(&schools).Error; err != nil {
		return nil, err
	}
	return schools, nil
}

// Create inserts a new school row and returns the persisted record with its
// generated ID and timestamps populated. If a school with the same name already
// exists (unique constraint violation), a 409 APIError is returned.
func (r *Repository) Create(principalID uint64, name, address, city, country, phone, email, website string) (*School, error) {
	s := &School{
		PrincipalID: principalID,
		Name:        name,
		Address:     address,
		City:        city,
		Country:     country,
		Phone:       phone,
		Email:       email,
		Website:     website,
	}
	if err := r.db.Create(s).Error; err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &middleware.APIError{
				Status:  http.StatusConflict,
				Code:    "SCHOOL_ALREADY_EXISTS",
				Message: "A school with that name already exists",
			}
		}
		return nil, err
	}
	return s, nil
}

// Update overwrites the mutable fields of the school identified by id. The read
// and write are wrapped in a transaction with a SELECT FOR UPDATE lock to
// prevent lost updates. Returns nil, nil when no row with that id exists.
func (r *Repository) Update(id uint64, name, address, city, country, phone, email, website string) (*School, error) {
	var s School
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, id).Error; err != nil {
			return err
		}
		result := tx.Model(&s).Updates(map[string]any{
			"name":    name,
			"address": address,
			"city":    city,
			"country": country,
			"phone":   phone,
			"email":   email,
			"website": website,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, &middleware.APIError{
				Status:  http.StatusConflict,
				Code:    "SCHOOL_ALREADY_EXISTS",
				Message: "A school with that name already exists",
			}
		}
		return nil, err
	}
	return r.FindByID(id)
}

// FindByIDs returns all schools whose primary key is in the given slice.
// Rows that do not exist are silently omitted from the result.
func (r *Repository) FindByIDs(ids []uint64) ([]*School, error) {
	var schools []*School
	if err := r.db.Where("id IN ?", ids).Find(&schools).Error; err != nil {
		return nil, err
	}
	return schools, nil
}

// SearchFilter holds optional predicates for the Search query. Zero values
// mean the field is not filtered.
type SearchFilter struct {
	Name        string // LIKE %name%
	City        string // exact match
	Country     string // exact match
	PrincipalID uint64 // exact match
}

// Search returns schools matching all non-zero fields in the filter.
func (r *Repository) Search(f SearchFilter) ([]*School, error) {
	q := r.db.Model(&School{})
	if f.Name != "" {
		q = q.Where("name LIKE ?", "%"+f.Name+"%")
	}
	if f.City != "" {
		q = q.Where("city = ?", f.City)
	}
	if f.Country != "" {
		q = q.Where("country = ?", f.Country)
	}
	if f.PrincipalID != 0 {
		q = q.Where("principal_id = ?", f.PrincipalID)
	}
	var schools []*School
	if err := q.Find(&schools).Error; err != nil {
		return nil, err
	}
	return schools, nil
}

// Delete removes the school row with the given id. Returns true if a row was
// deleted, false if no matching row existed, and an error for any database
// failure.
func (r *Repository) Delete(id uint64) (bool, error) {
	result := r.db.Delete(&School{}, id)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
