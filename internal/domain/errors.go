package domain

import "errors"

var (
	ErrInvalid   = errors.New("canalclear: invalid input")
	ErrNotFound  = errors.New("canalclear: not found")
	ErrConflict  = errors.New("canalclear: conflict")
	ErrForbidden = errors.New("canalclear: forbidden")
	ErrExpired   = errors.New("canalclear: expired")
	ErrState     = errors.New("canalclear: invalid state")
)
