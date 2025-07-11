package gcmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
)

type OutputBuffer struct {
	buf   *bytes.Buffer
	lines []string
	*sync.Mutex
}

func NewOutputBuffer() *OutputBuffer {
	out := &OutputBuffer{
		buf:   &bytes.Buffer{},
		lines: []string{},
		Mutex: &sync.Mutex{},
	}
	return out
}

func (rw *OutputBuffer) Write(p []byte) (n int, err error) {
	rw.Lock()
	n, err = rw.buf.Write(p)
	rw.Unlock()
	return
}

func (rw *OutputBuffer) Lines() []string {
	rw.Lock()
	s := bufio.NewScanner(rw.buf)
	for s.Scan() {
		rw.lines = append(rw.lines, s.Text())
	}
	rw.Unlock()
	return rw.lines
}

type Status struct {
	PID      int
	Complete bool
	Stdout   []string
	Stderr   []string
}

type Cmd struct {
	cmd *exec.Cmd
	env []string
	dir string
	*sync.Mutex
	started    bool
	stopped    bool
	done       bool
	final      bool
	status     Status
	statusChan chan Status
	doneChan   chan struct{}
	stdoutBuf  *OutputBuffer
	stderrBuf  *OutputBuffer
}

func NewCmd(name string, args ...string) *Cmd {
	cmd := exec.Command(name, args...)
	return &Cmd{
		cmd:        cmd,
		Mutex:      &sync.Mutex{},
		statusChan: make(chan Status, 1),
		doneChan:   make(chan struct{}),
		stdoutBuf:  NewOutputBuffer(),
		stderrBuf:  NewOutputBuffer(),
	}
}

func NewCmdWithCtx(ctx context.Context, name string, args ...string) *Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	return &Cmd{
		cmd:        cmd,
		Mutex:      &sync.Mutex{},
		statusChan: make(chan Status, 1),
		doneChan:   make(chan struct{}),
		stdoutBuf:  NewOutputBuffer(),
		stderrBuf:  NewOutputBuffer(),
	}
}

func (c *Cmd) SetDir(dir string) {
	c.dir = dir
}

func (c *Cmd) SetEnv(env []string) {
	c.env = env
}

func (c *Cmd) Start() <-chan Status {
	c.Lock()
	defer c.Unlock()
	go c.run()
	return c.statusChan
}

func (c *Cmd) Stop() error {
	c.Lock()
	defer c.Unlock()
	if c.stopped {
		return nil
	}
	c.stopped = true
	if c.statusChan == nil || !c.started {
		return fmt.Errorf("command not running")
	}
	if c.done {
		return nil
	}
	return terminateProcess(c.status.PID)
}

func (c *Cmd) Status() Status {
	c.Lock()
	defer c.Unlock()
	if c.statusChan == nil || !c.started {
		return c.status
	}

	if c.done {
		if !c.final {
			if c.stdoutBuf != nil {
				c.status.Stdout = c.stdoutBuf.Lines()
				c.stdoutBuf = nil
			}
			if c.stderrBuf != nil {
				c.status.Stderr = c.stderrBuf.Lines()
				c.stderrBuf = nil
			}
			c.final = true
		}
	} else {
		if c.stdoutBuf != nil {
			c.status.Stdout = c.stdoutBuf.Lines()
		}
		if c.stderrBuf != nil {
			c.status.Stderr = c.stderrBuf.Lines()
		}
	}
	return c.status
}

func (c *Cmd) run() {
	defer func() {
		c.statusChan <- c.Status() // unblocks Start if caller is waiting
		close(c.doneChan)
	}()
	setProcessGroupID(c.cmd)
	switch {
	case c.stdoutBuf != nil && c.stderrBuf != nil:
		c.cmd.Stdout = c.stdoutBuf
		c.cmd.Stderr = c.stderrBuf
	case c.stdoutBuf != nil && c.stderrBuf == nil:
		c.cmd.Stdout = c.stdoutBuf
		c.cmd.Stderr = c.stdoutBuf
	default:
		c.cmd.Stdout = nil
		c.cmd.Stderr = nil
	}
	if len(c.env) > 0 {
		c.cmd.Env = c.env
	}
	if c.dir != "" {
		c.cmd.Dir = c.dir
	}
	if err := c.cmd.Start(); err != nil {
		c.Lock()
		c.done = true
		c.Unlock()
		return
	}
	c.Lock()
	c.status.PID = c.cmd.Process.Pid
	c.started = true
	c.Unlock()
	err := c.cmd.Wait()
	signaled := false
	if err != nil && fmt.Sprintf("%T", err) == "*exec.ExitError" {
		exiterr := err.(*exec.ExitError)
		err = nil
		if waitStatus, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			if waitStatus.Signaled() {
				signaled = true
				err = fmt.Errorf(exiterr.Error())
			}
		}
	}
	c.Lock()
	if !c.stopped && !signaled {
		c.status.Complete = true
	}
	c.done = true
	c.Unlock()
}
