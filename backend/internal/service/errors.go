package service

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation failed")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

type UnavailableError struct {
	Dependency string
	Cause      error
}

func (e *UnavailableError) Error() string {
	if e.Cause == nil {
		return "service unavailable"
	}
	return fmt.Sprintf("service unavailable: %v", e.Cause)
}

func (e *UnavailableError) Unwrap() error {
	return e.Cause
}
