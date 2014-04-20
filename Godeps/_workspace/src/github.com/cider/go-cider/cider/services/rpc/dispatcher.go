// Copyright (c) 2013 The go-cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import (
	log "github.com/cihub/seelog"
	"io"
)

type dispatcher struct {
	transport Transport

	calls       map[RequestID]*RemoteCall
	streams     map[StreamTag]io.Writer
	progressCbs map[RequestID]func()

	executeCh   chan *executeCmd
	interruptCh chan *interruptCmd
	abandonCh   chan *RemoteCall
	termCh      chan struct{}
	termAckCh   chan struct{}

	taskManager *asyncTaskManager
	idPool      *idPool

	err error
}

func newDispatcher(transport Transport) *dispatcher {
	disp := &dispatcher{
		transport:   transport,
		calls:       make(map[RequestID]*RemoteCall),
		streams:     make(map[StreamTag]io.Writer),
		progressCbs: make(map[RequestID]func()),
		executeCh:   make(chan *executeCmd),
		interruptCh: make(chan *interruptCmd),
		abandonCh:   make(chan *RemoteCall),
		termCh:      make(chan struct{}),
		termAckCh:   make(chan struct{}),
		taskManager: newAsyncTaskManager(),
		idPool:      newIdPool(),
	}

	go disp.loop()
	return disp
}

// Public API ------------------------------------------------------------------

func (disp *dispatcher) NewRemoteCall(method string, args interface{}) *RemoteCall {
	return newRemoteCall(disp, method, args)
}

// Private API for Service -----------------------------------------------------

func (disp *dispatcher) shutdown() {
	select {
	case <-disp.termCh:
	default:
		close(disp.termCh)
	}
}

func (disp *dispatcher) terminated() <-chan struct{} {
	return disp.termAckCh
}

// Private API for RemoteCall --------------------------------------------------

type executeCmd struct {
	call  *RemoteCall
	errCh chan error
}

func (cmd *executeCmd) Type() int {
	return CmdCall
}

func (cmd *executeCmd) RequestId() RequestID {
	return cmd.call.id
}

func (cmd *executeCmd) Method() string {
	return cmd.call.method
}

func (cmd *executeCmd) Args() interface{} {
	return cmd.call.args
}

func (cmd *executeCmd) StdoutTag() *StreamTag {
	return cmd.call.stdoutTag
}

func (cmd *executeCmd) StderrTag() *StreamTag {
	return cmd.call.stderrTag
}

func (cmd *executeCmd) ErrorChan() chan<- error {
	return cmd.errCh
}

func (disp *dispatcher) executeRemoteCall(call *RemoteCall) {
	errCh := make(chan error, 1)
	select {
	case disp.executeCh <- &executeCmd{call, errCh}:
		if err := <-errCh; err != nil {
			disp.unregisterCall(call)
			call.resolve(nil, err)
		}
	case <-disp.termCh:
		call.resolve(nil, ErrTerminated)
	}
}

type interruptCmd struct {
	call  *RemoteCall
	errCh chan error
}

func (cmd *interruptCmd) Type() int {
	return CmdInterrupt
}

func (cmd *interruptCmd) TargetRequestId() RequestID {
	return cmd.call.id
}

func (cmd *interruptCmd) ErrorChan() chan<- error {
	return cmd.errCh
}

func (disp *dispatcher) interrupt(call *RemoteCall) (err error) {
	errCh := make(chan error, 1)

	select {
	case disp.interruptCh <- &interruptCmd{call, errCh}:
		err = <-errCh
	case <-disp.termCh:
		err = ErrTerminated
	}

	return
}

func (disp *dispatcher) abandon(call *RemoteCall) (err error) {
	if !call.interrupted() {
		if err := disp.interrupt(call); err != nil {
			return err
		}
	}

	select {
	case disp.abandonCh <- call:
	case <-disp.termCh:
		err = ErrTerminated
	}

	return
}

// Private methods -------------------------------------------------------------

func (disp *dispatcher) loop() {
	for {
		select {
		// executeCh contains enqueued remote calls initiated by this Service
		// instance. The calls are forwarded to the transport one by one.
		case cmd := <-disp.executeCh:
			// Make sure the call is not already interrupted.
			if cmd.call.interrupted() {
				cmd.call.resolve(nil, ErrInterrupted)
				continue
			}

			// Allocate necessary resources and register the call.
			disp.registerCall(cmd.call)

			// Dispatch the call.
			disp.transport.Call(cmd)

		// interruptCh contains outgoing interrupts, i.e. interrupts for
		// the remote requests initiated by this Service instance.
		case cmd := <-disp.interruptCh:
			disp.transport.Interrupt(cmd)

		// abandonCh contains calls that are to be dropped, i.e. unregistered
		// without really waiting for the reply to arrive.
		case call := <-disp.abandonCh:
			// Release the resources allocated by the call.
			disp.unregisterCall(call)

			// Resolve the call.
			call.resolve(nil, ErrInterrupted)

		// termCh is closed when shutdown is requested.
		case <-disp.termCh:
			log.Debug("Dispatcher: terminating")
			for {
				select {
				case <-disp.taskManager.Terminate():
					close(disp.termAckCh)
					log.Debug("Dispatcher: terminated")
					return

				// XXX: Not sure this is supposed to be discarded.
				case <-disp.transport.ProgressChan():
					continue

				case frame := <-disp.transport.StreamFrameChan():
					writer, ok := disp.streams[frame.TargetStreamTag()]
					if !ok {
						continue
					}

					writer.Write(frame.Payload())

				case reply := <-disp.transport.ReplyChan():
					call, ok := disp.calls[reply.TargetCallId()]
					if !ok {
						continue
					}
					call.resolve(reply, nil)
					disp.unregisterCall(call)
				}
			}

		// ProgressChan contains progress signals for the outgoing remote calls.
		case requestId := <-disp.transport.ProgressChan():
			handler, ok := disp.progressCbs[requestId]
			if !ok {
				// Drop progress of unknown requests.
				continue
			}

			disp.taskManager.Go(handler)

		// StreamFrameChan contains stream frames for the outgoing remote calls.
		case frame := <-disp.transport.StreamFrameChan():
			writer, ok := disp.streams[frame.TargetStreamTag()]
			if !ok {
				// Drop frames directed to unknown streams.
				continue
			}

			writer.Write(frame.Payload())

		// ReplyChan contains replies for the outgoing remote calls.
		case reply := <-disp.transport.ReplyChan():
			call, ok := disp.calls[reply.TargetCallId()]
			if !ok {
				// Drop replies to unknown calls.
				continue
			}

			call.resolve(reply, nil)
			// Free resources connected to the call.
			disp.unregisterCall(call)
		}
	}
}

func (disp *dispatcher) registerCall(call *RemoteCall) {
	// Assign the call an id and register it with the service.
	call.id = disp.allocateRequestId()
	disp.calls[call.id] = call

	// Register streams
	var (
		stdoutTag StreamTag
		stderrTag StreamTag
	)
	// Register the Stdout Writer that can be set by the user.
	if call.Stdout != nil {
		stdoutTag = disp.allocateStreamTag()
		call.stdoutTag = &stdoutTag
		disp.streams[stdoutTag] = newStreamWriter(call, call.Stdout)
	}
	// Register the Stderr Writer that can be set by the user.
	if call.Stderr != nil {
		stderrTag = disp.allocateStreamTag()
		call.stderrTag = &stderrTag
		disp.streams[stderrTag] = newStreamWriter(call, call.Stderr)
	}

	// Register OnProgress handler that can be set by the user.
	if call.OnProgress != nil {
		disp.progressCbs[call.id] = call.OnProgress
	}
}

func (disp *dispatcher) unregisterCall(call *RemoteCall) {
	// Unregister the call.
	delete(disp.calls, call.id)
	// Unregister the progress callback.
	delete(disp.progressCbs, call.id)
	// Release the id allocated by the call.
	disp.releaseRequestId(call.id)
	// Release the stream tags if any were allocated.
	if call.stdoutTag != nil {
		delete(disp.streams, *call.stdoutTag)
		disp.releaseStreamTag(*call.stdoutTag)
	}
	if call.stderrTag != nil {
		delete(disp.streams, *call.stderrTag)
		disp.releaseStreamTag(*call.stderrTag)
	}
}

func (disp *dispatcher) allocateRequestId() RequestID {
	return RequestID(disp.idPool.allocate())
}

func (disp *dispatcher) releaseRequestId(id RequestID) {
	disp.idPool.release(uint16(id))
}

func (disp *dispatcher) allocateStreamTag() StreamTag {
	return StreamTag(disp.idPool.allocate())
}

func (disp *dispatcher) releaseStreamTag(tag StreamTag) {
	disp.idPool.release(uint16(tag))
}

type streamWriter struct {
	call  *RemoteCall
	inner io.Writer
}

func newStreamWriter(call *RemoteCall, writer io.Writer) io.Writer {
	return &streamWriter{call, writer}
}

func (writer *streamWriter) Write(p []byte) (n int, err error) {
	n, err = writer.inner.Write(p)
	if err != nil {
		writer.call.Interrupt()
		writer.call.resolve(nil, err)
	}
	return
}
