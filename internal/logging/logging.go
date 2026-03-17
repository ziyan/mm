package logging

import (
	"os"

	"github.com/op/go-logging"
)

var Log = logging.MustGetLogger("mm")

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
