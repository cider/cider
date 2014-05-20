// Copyright (c) 2014 The cider AUTHORS
//
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package slave

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

type WorkspaceManager struct {
	root   string
	queues map[string]chan bool
	mu     *sync.Mutex
}

func newWorkspaceManager(root string) *WorkspaceManager {
	return &WorkspaceManager{
		root:   root,
		queues: make(map[string]chan bool),
		mu:     new(sync.Mutex),
	}
}

func (wm *WorkspaceManager) GetWorkspaceQueue(ws string) chan bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	q, ok := wm.queues[ws]
	if ok {
		return q
	}

	q = make(chan bool, 1)
	wm.queues[ws] = q
	return q
}

func (wm *WorkspaceManager) EnsureWorkspaceExists(repoURL *url.URL) (ws string, err error) {
	// Generate the project workspace path from the global workspace and
	// the repository URL so that the same repository names do not collide
	// unless the whole repository URLs are the same.
	ws = filepath.Join(wm.root, repoURL.Host, repoURL.Path, repoURL.Fragment)

	// Make sure the project workspace exists.
	err = ensureDirectoryExists(ws)
	return
}

func (mw *WorkspaceManager) SrcDir(workspace string) (srcDir string) {
	return filepath.Join(workspace, "src")
}

func (wm *WorkspaceManager) SrcDirExists(workspace string) (exists bool, err error) {
	return checkDirectoryExists(wm.SrcDir(workspace))
}

func checkDirectoryExists(path string) (exists bool, err error) {
	var info os.FileInfo
	info, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
			return
		}
		return
	}
	exists = true

	if !info.IsDir() {
		err = fmt.Errorf("not a directory: %v", path)
	}
	return
}

func ensureDirectoryExists(path string) (err error) {
	var exists bool
	exists, err = checkDirectoryExists(path)
	if exists || err != nil {
		return
	}
	return os.MkdirAll(path, 0750)
}
