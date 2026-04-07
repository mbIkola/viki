package logx

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

type Level int32

const (
	LevelError Level = iota
	LevelInfo
	LevelDebug
)

var currentLevel atomic.Int32

func init() {
	currentLevel.Store(int32(LevelInfo))
}

func Configure(configLevel string, quiet, verbose bool) error {
	lvl, err := DetermineLevel(configLevel, quiet, verbose)
	if err != nil {
		return err
	}
	SetLevel(lvl)
	Infof("[log] configured level=%s", lvl.String())
	return nil
}

func DetermineLevel(configLevel string, quiet, verbose bool) (Level, error) {
	if quiet && verbose {
		return LevelInfo, fmt.Errorf("--quiet and --verbose are mutually exclusive")
	}
	if quiet {
		return LevelError, nil
	}
	if verbose {
		return LevelDebug, nil
	}
	return ParseLevel(configLevel)
}

func ParseLevel(raw string) (Level, error) {
	norm := strings.ToUpper(strings.TrimSpace(raw))
	if norm == "" {
		return LevelInfo, nil
	}
	switch norm {
	case "ERROR":
		return LevelError, nil
	case "INFO":
		return LevelInfo, nil
	case "DEBUG":
		return LevelDebug, nil
	default:
		return LevelInfo, fmt.Errorf("unsupported log level %q (expected ERROR|INFO|DEBUG)", raw)
	}
}

func SetLevel(level Level) {
	currentLevel.Store(int32(level))
}

func GetLevel() Level {
	return Level(currentLevel.Load())
}

func (l Level) String() string {
	switch l {
	case LevelError:
		return "ERROR"
	case LevelInfo:
		return "INFO"
	case LevelDebug:
		return "DEBUG"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", l)
	}
}

func Errorf(format string, args ...any) {
	logf(LevelError, format, args...)
}

func Infof(format string, args ...any) {
	logf(LevelInfo, format, args...)
}

func Debugf(format string, args ...any) {
	logf(LevelDebug, format, args...)
}

func logf(level Level, format string, args ...any) {
	if level > GetLevel() {
		return
	}
	log.Printf("[%s] %s", level.String(), fmt.Sprintf(format, args...))
}
