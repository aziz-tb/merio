package dbutil

import "github.com/lib/pq"

// IsForeignKeyError checks if the given error is a PostgreSQL foreign key violation (error code 23503)
func IsForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23503"
	}
	return false
}

// IsUniqueViolationError checks if the given error is a PostgreSQL unique violation (error code 23505)
func IsUniqueViolationError(err error) bool {
	if err == nil {
		return false
	}
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23505"
	}
	return false
}

// IsTableNotExistError checks if the given error is a PostgreSQL table does not exist error (error code 42P01)
func IsTableNotExistError(err error) bool {
	if err == nil {
		return false
	}
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "42P01"
	}
	return false
}