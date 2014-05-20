// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package publisher

import (
	log "github.com/meeko/meekod/broker/services/logging"
	ex "github.com/tchap/go-exchange/exchange"
)

//------------------------------------------------------------------------------
// Publisher implements logging.Exchange
//------------------------------------------------------------------------------

type Publisher struct {
	ex *ex.Exchange
}

func New() *Publisher {
	return &Publisher{
		ex: ex.New(),
	}
}

type RecordHandler func(level log.Level, record []byte)

func (pub *Publisher) Subscribe(srcApp string, level log.Level, handler RecordHandler) (ex.Handle, error) {
	return pub.ex.Subscribe(topic(srcApp, level), func(t ex.Topic, e ex.Event) {
		handler(log.Level(t[len(t)-1]), e.([]byte))
	})
}

func (pub *Publisher) Unsubscribe(handle ex.Handle) error {
	return pub.ex.Unsubscribe(handle)
}

func (pub *Publisher) Log(srcApp string, level log.Level, message []byte) error {
	return pub.ex.Publish(topic(srcApp, level), message)
}

func (pub *Publisher) Flush() {
	return
}

func (pub *Publisher) Close() {
	pub.ex.Terminate()
}

const (
	levelUnset    = byte(log.LevelUnset)
	levelTrace    = byte(log.LevelTrace)
	levelDebug    = byte(log.LevelDebug)
	levelInfo     = byte(log.LevelInfo)
	levelWarn     = byte(log.LevelWarn)
	levelError    = byte(log.LevelError)
	levelCritical = byte(log.LevelCritical)
)

func topic(srcApp string, level log.Level) ex.Topic {
	srcAppBytes := []byte(srcApp)
	topic := make([]byte, len(srcAppBytes)+int(log.LevelCritical)+1)
	copy(topic, srcAppBytes)
	topic = topic[:len(srcAppBytes)]

	switch level {
	case log.LevelUnset:
		topic = append(topic,
			levelUnset)
	case log.LevelTrace:
		topic = append(topic,
			levelUnset,
			levelTrace)
	case log.LevelDebug:
		topic = append(topic,
			levelUnset,
			levelTrace,
			levelDebug)
	case log.LevelInfo:
		topic = append(topic,
			levelUnset,
			levelTrace,
			levelDebug,
			levelInfo)
	case log.LevelWarn:
		topic = append(topic,
			levelUnset,
			levelTrace,
			levelDebug,
			levelInfo,
			levelWarn)
	case log.LevelError:
		topic = append(topic,
			levelUnset,
			levelTrace,
			levelDebug,
			levelInfo,
			levelWarn,
			levelError)
	case log.LevelCritical:
		topic = append(topic,
			levelUnset,
			levelTrace,
			levelDebug,
			levelInfo,
			levelWarn,
			levelError,
			levelCritical)
	default:
		panic("unknown log level")
	}

	return topic
}
