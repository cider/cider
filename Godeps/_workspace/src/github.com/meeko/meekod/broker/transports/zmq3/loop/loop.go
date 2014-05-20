// Copyright (c) 2013 The meeko AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package loop

import (
	"crypto/rand"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/meeko/meekod/broker/log"

	zmq "github.com/pebbe/zmq3"
)

//------------------------------------------------------------------------------
// MessageLoop
//------------------------------------------------------------------------------

type MessageHandler func(msg [][]byte)

type PollItem struct {
	Socket   *zmq.Socket
	Callback MessageHandler
}

type PollItems []PollItem

type MessageLoop struct {
	// Internal loop state
	state LoopState

	// User-defined poll items
	items PollItems

	// Internal inproc 0MQ sockets
	sendPipe  *zmq.Socket
	closePipe *zmq.Socket

	// To make access to 0MQ sockets and the loop state thread-safe.
	mu *sync.Mutex

	// To signal Close() that ListenAndServe() had returned.
	closeAckCh chan struct{}
}

type LoopState byte

const (
	stateInit LoopState = iota
	stateRunning
	stateTerminated
)

var stateStrings = [...]string{
	"INIT",
	"RUNNING",
	"TERMINATED",
}

func (state LoopState) String() string {
	return stateStrings[int(state)]
}

func New(items PollItems) *MessageLoop {
	if len(items) == 0 {
		panic(ErrEmptyPollItems)
	}
	if len(items) > 255 {
		panic(ErrTooManyPollItems)
	}

	return &MessageLoop{
		items:      items,
		mu:         new(sync.Mutex),
		closeAckCh: make(chan struct{}),
	}
}

func (loop *MessageLoop) ListenAndServe() error {
	loop.mu.Lock()
	if loop.state != stateInit {
		loop.mu.Unlock()
		return &ErrInvalidState{stateInit, loop.state}
	}

	defer func() {
		// Take the lock in case there is a panic. This must be here because
		// otherwise we would be unlocking and already unlocked mutex.
		//
		// XXX: THIS METHOD IS UNGLY LIKE HELL
		r := recover()
		if r != nil {
			loop.mu.Lock()
		}
		defer loop.mu.Unlock()

		// Close the user-defined sockets on exit.
		for _, item := range loop.items {
			item.Socket.Close()
		}

		// Set the internal state to TERMINATED.
		loop.state = stateTerminated

		// Unblock Close().
		close(loop.closeAckCh)

		// Forward the panic in case there was any.
		if r != nil {
			panic(r)
		}
	}()

	// Build the pipe for sending messages to the sockets that are being polled.
	var err error
	sendPipeIn, err := zmq.NewSocket(zmq.PAIR)
	if err != nil {
		return err
	}
	defer sendPipeIn.Close()

	sendPipeOut, err := zmq.NewSocket(zmq.PAIR)
	if err != nil {
		return err
	}
	defer sendPipeOut.Close()

	ep, err := randomInprocEndpoint()
	if err != nil {
		return err
	}

	err = sendPipeOut.Bind(ep)
	if err != nil {
		return err
	}

	err = sendPipeIn.Connect(ep)
	if err != nil {
		return err
	}

	loop.sendPipe = sendPipeIn

	// Build the pipe for sending the termination signal.
	closePipeIn, err := zmq.NewSocket(zmq.PAIR)
	if err != nil {
		return err
	}
	defer closePipeIn.Close()

	closePipeOut, err := zmq.NewSocket(zmq.PAIR)
	if err != nil {
		return err
	}
	defer closePipeOut.Close()

	ep, err = randomInprocEndpoint()
	if err != nil {
		return err
	}

	err = closePipeOut.Bind(ep)
	if err != nil {
		return err
	}

	err = closePipeIn.Connect(ep)
	if err != nil {
		return err
	}

	loop.closePipe = closePipeIn

	// Set up the poller.
	poller := zmq.NewPoller()
	poller.Add(sendPipeOut, zmq.POLLIN)
	poller.Add(closePipeOut, zmq.POLLIN)
	for _, item := range loop.items {
		poller.Add(item.Socket, zmq.POLLIN)
	}

	// Set the internal state to RUNNING and release the loop lock.
	loop.state = stateRunning
	loop.mu.Unlock()

	// Start processing messages.
	var (
		polled []zmq.Polled
		msg    [][]byte
	)
	for {
		polled, err = poller.PollAll(-1)
		if err != nil {
			if err.Error() == "interrupted system call" {
				log.Debug("zmq3: Received EINTR, but ignoring...")
				continue
			}
			log.Critical("zmq3: Message loop crashed: %s", err)
			break
		}

		// Message sent into the loop, forward it to the right socket.
		if polled[0].Events != 0 {
			msg, err = sendPipeOut.RecvMessageBytes(0)
			if err != nil {
				break
			}

			// Here we assume the user is not stupid enough to trigger a panic
			// by sending empty socket offset.
			i := msg[0][0]
			_, err = loop.items[i].Socket.SendMessage(msg[1:])
			if err != nil {
				break
			}
		}

		// Termination signal received, shut down.
		if polled[1].Events != 0 {
			break
		}

		// Finally, check the user poll items as well.
		for i := 2; i < len(polled); i++ {
			if polled[i].Events != 0 {
				msg, err = loop.items[i-2].Socket.RecvMessageBytes(0)
				if err != nil {
					break
				}

				loop.invokeCallback(i-2, msg)
			}
		}
	}

	// Lock the loop to perform cleanup atomically, without getting any
	// additional messages from other endpoints.
	loop.mu.Lock()

	// Drain the send pipe. This is necessary to make the loop work in
	// a predictable way, i.e. messages sent using DispatchMessage before
	// calling Close() are really processed and not just silently dropped.
	for {
		msg, ex := sendPipeOut.RecvMessageBytes(zmq.DONTWAIT)
		if ex != nil {
			if ex.Error() != "resource temporarily unavailable" {
				log.Errorf("zmq3: Message loop cleanup failed: %s", ex)
			}
			break
		}

		// Here we assume the user is not stupid enough to trigger a panic.
		i := msg[0][0]
		_, ex = loop.items[i].Socket.SendMessage(msg[1:])
		if ex != nil {
			log.Errorf("zmq3: Message loop cleanup failed: %s", ex)
			break
		}
	}

	return err
}

func (loop *MessageLoop) invokeCallback(i int, msg [][]byte) {
	// Do not propagate panics. A user-defined callback should not be allowed
	// to bring the whole message loop down.
	defer func() {
		if r := recover(); r != nil {
			st := make([]byte, 4096)
			n := runtime.Stack(st, false)
			log.Debugf("zmq3: Poll loop message callback panicked: %v\n%v", r, string(st[:n]))
		}
	}()
	// Call the relevant socket callback.
	loop.items[i].Callback(msg)
}

func (loop *MessageLoop) DispatchMessage(itemIndex byte, msg [][]byte) error {
	if len(msg) == 0 {
		return ErrEmptyMessage
	}

	loop.mu.Lock()
	defer loop.mu.Unlock()

	if loop.state != stateRunning {
		return &ErrInvalidState{stateRunning, loop.state}
	}

	_, err := loop.sendPipe.SendBytes([]byte{itemIndex}, zmq.SNDMORE|zmq.DONTWAIT)
	if err != nil {
		return err
	}

	for i := 0; i < len(msg)-1; i++ {
		_, err = loop.sendPipe.SendBytes(msg[i], zmq.SNDMORE|zmq.DONTWAIT)
		if err != nil {
			return err
		}
	}

	_, err = loop.sendPipe.SendBytes(msg[len(msg)-1], zmq.DONTWAIT)
	if err != nil {
		return err
	}

	return nil
}

func (loop *MessageLoop) Close() error {
	loop.mu.Lock()
	if loop.state != stateRunning {
		loop.mu.Unlock()
		return &ErrInvalidState{stateRunning, loop.state}
	}

	_, err := loop.closePipe.Send("KONEC VYSILANI", 0)
	if err != nil {
		loop.mu.Unlock()
		return err
	}

	// We need to hold the lock until the internal loop returns.
	loop.mu.Unlock()
	<-loop.closeAckCh
	return nil
}

// Utilities -------------------------------------------------------------------

func randomInprocEndpoint() (string, error) {
	rnd := make([]byte, 10)
	if _, err := rand.Read(rnd); err != nil {
		return "", err
	}
	return "inproc://rnd" + string(rnd), nil
}

// Errors ----------------------------------------------------------------------

var (
	ErrEmptyPollItems   = errors.New("Empty PollItems")
	ErrEmptyMessage     = errors.New("Empty message")
	ErrTooManyPollItems = errors.New("No mobre than 255 PollItems are supported")
)

type ErrInvalidState struct {
	Expected LoopState
	Current  LoopState
}

func (err *ErrInvalidState) Error() string {
	return fmt.Sprintf("Message loop state invalid: expected %s, is %s", err.Expected, err.Current)
}
