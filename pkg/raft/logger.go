package raft

import "log"

// Log levels
const (
	// Error only
	Error = 1
	// Warning and error
	Warning = 2
	// Information, warning and error
	Information = 3
	// All
	Trace = 4
)

var logger = log.New(log.Writer(), log.Prefix(), log.Flags())
var logLevel = Information

func writeLog(level int, format string, v ...interface{}) {
	if level <= logLevel {
		logger.Printf(format, v...)
	}
}

func writeError(format string, v ...interface{}) {
	writeLog(Error, format, v...)
}

func writeWarning(format string, v ...interface{}) {
	writeLog(Warning, format, v...)
}

func writeInfo(format string, v ...interface{}) {
	writeLog(Information, format, v...)
}

func writeTrace(format string, v ...interface{}) {
	writeLog(Trace, format, v...)
}
