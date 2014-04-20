// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package vcsutil

import (
	"bytes"
	"github.com/cider/cider/apps/utils/executil"
	"net/url"
	"os/exec"
)

type gitVCS struct {
	scheme string
}

func newGitVCS(scheme string) VCS {
	return &gitVCS{scheme}
}

func (vcs *gitVCS) Clone(repoURL *url.URL, srcDir string, ctx ActionContext) error {
	// Assemble clone URL.
	var buf bytes.Buffer
	buf.WriteString(vcs.scheme)
	buf.WriteString("://")
	if repoURL.User != nil {
		buf.WriteString(repoURL.User.String())
		buf.WriteString("@")
	}
	buf.WriteString(repoURL.Host)
	buf.WriteByte('/')
	buf.WriteString(repoURL.Path)

	// Assemble git flags and arguments.
	branch := repoURL.Fragment
	if branch == "" {
		branch = "master"
	}
	args := []string{"clone", "--branch", branch, "--single-branch"}
	args = append(args, buf.String(), srcDir)

	// Initialise the command.
	cmd := exec.Command("git", args...)
	cmd.Stderr = ctx.Stderr()
	cmd.Stdout = ctx.Stdout()

	// Run the command.
	return executil.Run(cmd, ctx.Interrupted())
}

func (vcs *gitVCS) Pull(repoURL *url.URL, srcDir string, ctx ActionContext) error {
	branch := repoURL.Fragment
	if branch == "" {
		branch = "master"
	}

	// Fetch
	cmd := exec.Command("git", "fetch", "origin", branch)
	cmd.Dir = srcDir
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()

	if err := executil.Run(cmd, ctx.Interrupted()); err != nil {
		return err
	}

	// Checkout
	cmd = exec.Command("git", "checkout", branch)
	cmd.Dir = srcDir
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()

	if err := executil.Run(cmd, ctx.Interrupted()); err != nil {
		return err
	}

	// Merge
	cmd = exec.Command("git", "merge", "origin/"+branch)
	cmd.Dir = srcDir
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()

	return executil.Run(cmd, ctx.Interrupted())
}
