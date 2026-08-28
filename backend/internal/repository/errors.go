package repository

import "errors"

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists is returned when a create/update would cause a name collision.
	ErrAlreadyExists = errors.New("already exists")
)
