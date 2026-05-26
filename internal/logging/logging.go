package logging

import (
	"os"

	"github.com/op/go-logging"
)

// Per-package logger declaration required by the Mujin coding standard
// (mulint_log). The logging package itself doesn't use this for output;
// consumer packages declare their own `var log = logging.MustGetLogger(...)`.
var log = logging.MustGetLogger("logging") //nolint:unused

var format = logging.MustStringFormatter(
	"%{color}%{time:2006-01-02 15:04:05.000} %{module} [%{level}] %{message}%{color:reset}",
)

func Setup(levelName string) {
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	formatted := logging.NewBackendFormatter(backend, format)
	leveled := logging.AddModuleLevel(formatted)

	level, err := logging.LogLevel(levelName)
	if err != nil {
		level = logging.INFO
	}
	leveled.SetLevel(level, "")
	logging.SetBackend(leveled)
}
