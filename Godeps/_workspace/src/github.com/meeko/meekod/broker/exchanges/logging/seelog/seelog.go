// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package seelog

import (
	"github.com/meeko/meekod/broker/services/logging"

	slog "github.com/cihub/seelog"
)

var Default = &Logger{slog.Default}

//------------------------------------------------------------------------------
// Logger implements logging.Handler
//------------------------------------------------------------------------------

type Logger struct {
	slog.LoggerInterface
}

func (logger *Logger) Log(srcApp string, level logging.Level, message []byte) error {
	msg := string(message)

	switch level {
	case logging.LevelTrace:
		logger.Tracef("[%s] %s", srcApp, msg)
	case logging.LevelDebug:
		logger.Debugf("[%s] %s", srcApp, msg)
	case logging.LevelInfo:
		logger.Infof("[%s] %s", srcApp, msg)
	case logging.LevelWarn:
		logger.Warnf("[%s] %s", srcApp, msg)
	case logging.LevelCritical:
		logger.Criticalf("[%s] %s", srcApp, msg)
	}

	return nil
}
