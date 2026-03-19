// Package pkg/log provides unified log format: [timestamp] [level] [action_id] message
package log

import (
	"fmt"
	"log"
	"os"
	"time"
)

var level = "info"

func SetLevel(l string) { level = l }

func shouldLog(l string) bool {
	order := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3, "fatal": 4}
	return order[l] >= order[level]
}

func formatLine(lev, actionID, msg string) string {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	if actionID != "" {
		return fmt.Sprintf("[%s] [%s] [%s] %s", ts, lev, actionID, msg)
	}
	return fmt.Sprintf("[%s] [%s] [] %s", ts, lev, msg)
}

func Infof(actionID, msg string, args ...any) {
	if !shouldLog("info") {
		return
	}
	log.Output(2, formatLine("info", actionID, fmt.Sprintf(msg, args...)))
}

func Warnf(actionID, msg string, args ...any) {
	if !shouldLog("warn") {
		return
	}
	log.Output(2, formatLine("warn", actionID, fmt.Sprintf(msg, args...)))
}

func Errorf(actionID, msg string, args ...any) {
	if !shouldLog("error") {
		return
	}
	log.Output(2, formatLine("error", actionID, fmt.Sprintf(msg, args...)))
}

func Debugf(actionID, msg string, args ...any) {
	if !shouldLog("debug") {
		return
	}
	log.Output(2, formatLine("debug", actionID, fmt.Sprintf(msg, args...)))
}

func Fatal(msg string, args ...any) {
	log.Output(2, formatLine("fatal", "", fmt.Sprintf(msg, args...)))
	os.Exit(1)
}
