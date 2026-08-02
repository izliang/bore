package db

import "errors"

// Sentinel errors returned by the db package.
var (
	// ErrDSNRequired is returned when Config.DSN is empty.
	ErrDSNRequired = errors.New("db: DSN is required")

	// ErrClosed is returned when operating on a closed or nil *gorm.DB.
	ErrClosed = errors.New("db: database connection is closed")
)
