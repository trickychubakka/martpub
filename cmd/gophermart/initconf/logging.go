package initconf

import (
	"fmt"
	"log/slog"
	"os"
)

var Reset = "\033[0m"
var RedBG = "\033[41m"
var OrangeBG = "\033[43m"
var BlueBG = "\033[44m"
var GreenBG = "\033[42m"

// SetLogger функция настройки logger-а
func SetLogger(conf *Config) (*slog.Logger, error) {
	local := conf.LogConf.LocalRun
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: conf.LogConf.LogLevel,

		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if !local {
					return a
				}
				level := a.Value.Any().(slog.Level)

				if level == slog.LevelDebug {
					fmt.Print(BlueBG + "DEBUG" + Reset + " ")
				} else if level == slog.LevelInfo {
					fmt.Print(GreenBG + "INFO" + Reset + " ")
				} else if level == slog.LevelWarn {
					fmt.Print(OrangeBG + "WARN" + Reset + " ")
				} else {
					fmt.Print(RedBG + "ERROR" + Reset + " ")
				}
				return slog.Attr{}
			} else if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006/01/02 15:04:05") + " ")
				fmt.Print(a.Value)
				return slog.Attr{}
			} else if a.Key == slog.MessageKey {
				if local {
					fmt.Print(a.Value, " ")
					return slog.Attr{}
				}
			}
			return a
		},
	}))
	conf.LogConf.Logger = logger
	return logger, nil
}
