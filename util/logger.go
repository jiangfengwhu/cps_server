package util

import (
	"io"
	"log"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

func InitLogger(logDir string) *lumberjack.Logger {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("failed to create log directory: %v", err)
	}

	lj := &lumberjack.Logger{
		Filename:   logDir + "/cps.log",
		MaxSize:    10, // MB
		MaxBackups: 1,
		MaxAge:     7, // days
		Compress:   true,
	}

	multiWriter := io.MultiWriter(os.Stdout, lj)
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	return lj
}
