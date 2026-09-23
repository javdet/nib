package kbsettings

import "errors"

// ErrInvalidJSON is returned when the settings file does not parse.
var ErrInvalidJSON = errors.New("invalid knowledge settings JSON")
