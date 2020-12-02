package util

import "log"

// Log levels
const (
	// levelError only
	levelError = 1
	// levelWarning and error
	levelWarning = 2
	// levelInfo, warning and error
	levelInfo = 3
	// All
	levelTrace = 4
)

// raft logger and log level
var logger = log.New(log.Writer(), log.Prefix(), log.Flags())
var logLevel = levelInfo

// SetLogLevel sets log level
func SetLogLevel(level int) {
	if level < levelError {
		level = levelError
	}
	if level > levelTrace {
		level = levelTrace
	}

	logLevel = level
}

// WriteLog writes an log entry if its level is lower than logLevel, otherwise it's ignored
func WriteLog(level int, format string, v ...interface{}) {
	if level <= logLevel {
		logger.Printf(format, v...)
	}
}

// WriteError writes an error log
func WriteError(format string, v ...interface{}) {
	WriteLog(levelError, format, v...)
}

// WriteWarning writes a warning log
func WriteWarning(format string, v ...interface{}) {
	WriteLog(levelWarning, format, v...)
}

// WriteInfo writes a information
func WriteInfo(format string, v ...interface{}) {
	WriteLog(levelInfo, format, v...)
}

// WriteTrace writes traces and debug information
func WriteTrace(format string, v ...interface{}) {
	WriteLog(levelTrace, format, v...)
}

// Panicf is equivalent to l.Printf() followed by a call to panic().
func Panicf(format string, v ...interface{}) {
	logger.Panicf(format, v...)
}

// Panicln is equivalent to l.Println() followed by a call to panic().
func Panicln(v ...interface{}) {
	logger.Panicln(v...)
}
