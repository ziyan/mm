package config

import "github.com/op/go-logging"

// Per-package logger declaration required by the project coding standard
// (mulint_log). Currently unused — kept under //nolint:unused so callers
// can add log.Debug/log.Errorf calls without further plumbing.
var log = logging.MustGetLogger("config") //nolint:unused
