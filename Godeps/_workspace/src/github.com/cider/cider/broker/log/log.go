// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package log

import (
	"errors"
	"io"

	"github.com/cihub/seelog"
)

var logger seelog.LoggerInterface

func init() {
	DisableLog()
}

func DisableLog() {
	logger = seelog.Disabled
}

func UseLogger(newLogger seelog.LoggerInterface) {
	newLogger.SetAdditionalStackDepth(1)
	logger = newLogger
}

func SetLogWriter(writer io.Writer) error {
	if writer == nil {
		return errors.New("Nil writer not allowed")
	}

	newLogger, err := seelog.LoggerFromWriterWithMinLevel(writer, seelog.TraceLvl)
	if err != nil {
		return err
	}

	UseLogger(newLogger)
	return nil
}

func Tracef(format string, params ...interface{}) {
	logger.Tracef(format, params...)
}

func Debugf(format string, params ...interface{}) {
	logger.Debugf(format, params...)
}

func Infof(format string, params ...interface{}) {
	logger.Infof(format, params...)
}

func Warnf(format string, params ...interface{}) error {
	return logger.Warnf(format, params...)
}

func Errorf(format string, params ...interface{}) error {
	return logger.Errorf(format, params...)
}

func Criticalf(format string, params ...interface{}) error {
	return logger.Criticalf(format, params...)
}

func Trace(v ...interface{}) {
	logger.Trace(v...)
}

func Debug(v ...interface{}) {
	logger.Debug(v...)
}

func Info(v ...interface{}) {
	logger.Info(v...)
}

func Warn(v ...interface{}) error {
	return logger.Warn(v...)
}

func Error(v ...interface{}) error {
	return logger.Error(v...)
}

func Critical(v ...interface{}) error {
	return logger.Critical(v...)
}

func Close() {
	logger.Close()
}

func Closed() bool {
	return logger.Closed()
}

func Flush() {
	logger.Flush()
}
