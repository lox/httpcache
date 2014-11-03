package httpcache

import (
	"log"
	"os"
)

const (
	ansiRed   = "\x1b[31;1m"
	ansiReset = "\x1b[0m"
)

var DebugLogging = false

func Debugf(format string, args ...interface{}) {
	if DebugLogging {
		log.Printf(format, args...)
	}
}

func Errorf(format string, args ...interface{}) {
	log.Printf(ansiRed+"✗ "+format+ansiReset, args)
}

func Fatal(args ...interface{}) {
	Errorf("%#v", args...)
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	Errorf(format, args...)
	os.Exit(1)
}
