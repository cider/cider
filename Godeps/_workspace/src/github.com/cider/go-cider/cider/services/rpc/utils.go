// Copyright (c) 2013 The go-cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package rpc

import "sync/atomic"

// asynchronous task manager ---------------------------------------------------

type asyncTaskManager struct {
	numRunningTasks int32
	taskReturnedCh  chan bool
	termCh          chan struct{}
	termAckCh       chan struct{}
}

func newAsyncTaskManager() *asyncTaskManager {
	return &asyncTaskManager{
		taskReturnedCh: make(chan bool),
		termCh:         make(chan struct{}),
		termAckCh:      make(chan struct{}),
	}
}

func (mgr *asyncTaskManager) Go(task func()) {
	select {
	case <-mgr.termCh:
		panic("asynchronous task manager is shutting down")
	case <-mgr.termAckCh:
		panic("asynchronous task manager has been terminated")
	default:
	}

	atomic.AddInt32(&mgr.numRunningTasks, 1)
	go func() {
		defer func() {
			recover()
			running := atomic.AddInt32(&mgr.numRunningTasks, -1)

			select {
			case <-mgr.termCh:
				if running == 0 {
					close(mgr.termAckCh)
				}
			default:
			}
		}()
		task()
	}()
}

func (mgr *asyncTaskManager) Terminate() <-chan struct{} {
	select {
	case <-mgr.termCh:
	default:
		close(mgr.termCh)
	}

	if atomic.LoadInt32(&mgr.numRunningTasks) == 0 {
		select {
		case <-mgr.termAckCh:
		default:
			close(mgr.termAckCh)
		}
	}

	return mgr.termAckCh
}

// ID pool ---------------------------------------------------------------------

type idPool struct {
	next      uint16
	allocated map[uint16]bool
}

func newIdPool() *idPool {
	return &idPool{
		allocated: make(map[uint16]bool),
	}
}

func (pool *idPool) allocate() uint16 {
	overflow := pool.next
	pool.next++
	for ; pool.next != overflow; pool.next++ {
		if _, ok := pool.allocated[pool.next]; !ok {
			pool.allocated[pool.next] = true
			return pool.next
		}
	}
	panic("ID pool depleted")
}

func (pool *idPool) release(id uint16) {
	delete(pool.allocated, id)
}
