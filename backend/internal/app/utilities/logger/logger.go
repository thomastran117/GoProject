package logger

import (
	"fmt"
	"os"
	"time"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"

	colourDebug = "\033[36m"
	colourInfo  = "\033[32m"
	colourWarn  = "\033[33m"
	colourError = "\033[31m"
	colourFatal = "\033[35m"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var MinLevel = LevelDebug

func timestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func log(colour, label string, level Level, msg string, args ...any) {
	if level < MinLevel {
		return
	}
	formatted := msg
	if len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}
	out := os.Stdout
	if level >= LevelError {
		out = os.Stderr
	}
	fmt.Fprintf(out, "%s%s%s [%s%-5s%s] %s\n",
		bold, timestamp(), reset,
		colour, label, reset,
		formatted,
	)
}

func Debug(msg string, args ...any) { log(colourDebug, "DEBUG", LevelDebug, msg, args...) }
func Info(msg string, args ...any)  { log(colourInfo, "INFO", LevelInfo, msg, args...) }
func Warn(msg string, args ...any)  { log(colourWarn, "WARN", LevelWarn, msg, args...) }
func Error(msg string, args ...any) { log(colourError, "ERROR", LevelError, msg, args...) }

func Fatal(msg string, args ...any) {
	log(colourFatal, "FATAL", LevelFatal, msg, args...)
	os.Exit(1)
}
