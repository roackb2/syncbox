package syncbox

import (
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// global variables to control overall logging behavior
const (
	GlobalLogInfo    = true
	GlobalLogError   = true
	GlobalLogDebug   = true
	DefaultAppPrefix = "syncbox"
)

// Logger logs error, info and debug messages
type Logger struct {
	infoLogger  *log.Logger
	errorLogger *log.Logger
	debugLogger *log.Logger
	doLogInfo   bool
	doLogError  bool
	doLogDebug  bool
}

// NewLogger instantiate Logger
func NewLogger(prefix string, logInfo bool, logError bool, logDebug bool) *Logger {
	return &Logger{
		debugLogger: log.New(os.Stdout, "syncbox: ", log.LstdFlags),
		infoLogger:  log.New(os.Stdout, "syncbox: ", log.LstdFlags),
		errorLogger: log.New(os.Stderr, "syncbox: ", log.LstdFlags),
		doLogInfo:   logInfo,
		doLogError:  logError,
		doLogDebug:  logDebug,
	}
}

// LogError logs error messages
func (l *Logger) LogError(format string, v ...interface{}) {
	if l.doLogError {
		_, path, line, _ := runtime.Caller(1)
		elements := strings.Split(path, "/")
		file := elements[len(elements)-1]
		if v != nil && len(v) != 0 {
			l.errorLogger.Printf("error "+file+" "+strconv.Itoa(line)+": "+format, v...)
		} else {
			l.errorLogger.Printf("error " + file + " " + strconv.Itoa(line) + ": " + format)
		}
		debug.PrintStack()
	}
}

// LogInfo logs info messages
func (l *Logger) LogInfo(format string, v ...interface{}) {
	if l.doLogInfo {
		_, path, line, _ := runtime.Caller(1)
		elements := strings.Split(path, "/")
		file := elements[len(elements)-1]
		if v != nil && len(v) != 0 {
			l.infoLogger.Printf("info "+file+" "+strconv.Itoa(line)+": "+format, v...)
		} else {
			l.infoLogger.Printf("info " + file + " " + strconv.Itoa(line) + ": " + format)
		}
	}
}

// LogDebug logs debug messages
func (l *Logger) LogDebug(format string, v ...interface{}) {
	if l.doLogDebug {
		_, path, line, _ := runtime.Caller(1)
		elements := strings.Split(path, "/")
		file := elements[len(elements)-1]
		if v != nil && len(v) != 0 {
			l.infoLogger.Printf("debug "+file+" "+strconv.Itoa(line)+": "+format, v...)
		} else {
			l.infoLogger.Printf("debug " + file + " " + strconv.Itoa(line) + ": " + format)
		}
	}
}
