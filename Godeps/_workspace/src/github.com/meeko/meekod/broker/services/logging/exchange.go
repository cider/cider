// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package logging

import (
	"errors"
	"github.com/meeko/meekod/broker"
	"strings"
)

type Level byte

const (
	LevelUnset Level = iota
	LevelTrace
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelCritical
)

var level2string = [...]string{
	LevelUnset:    "UNSET",
	LevelTrace:    "TRACE",
	LevelDebug:    "DEBUG",
	LevelInfo:     "INFO",
	LevelWarn:     "WARNING",
	LevelError:    "ERROR",
	LevelCritical: "CRITICAL",
}

func (level Level) String() string {
	return level2string[int(level)]
}

var string2level = map[string]Level{
	"UNSET":    LevelUnset,
	"TRACE":    LevelTrace,
	"DEBUG":    LevelDebug,
	"INFO":     LevelInfo,
	"WARNING":  LevelWarn,
	"ERROR":    LevelError,
	"CRITICAL": LevelCritical,
}

func ParseLogLevel(levelString string) (Level, error) {
	level, ok := string2level[strings.ToUpper(levelString)]
	if !ok {
		return LevelUnset, errors.New("unknown log level string")
	}

	return level, nil
}

type Endpoint broker.Endpoint

type Exchange interface {
	Log(identity string, level Level, message []byte) error
	Flush()
	Close()
}
