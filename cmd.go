package gcmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
)

type OutputBuffer struct {
	buf []byte
	*sync.Mutex
}

func NewOutputBuffer() *OutputBuffer {
	out := &OutputBuffer{
		buf:   []byte{},
		Mutex: &sync.Mutex{},
	}
	return out
}

func (rw *OutputBuffer) Write(p []byte) (n int, err error) {
	rw.Lock()
	rw.buf = p
	rw.Unlock()
	return len(p), nil
}

func (rw *OutputBuffer) Latest() string {
	rw.Lock()
	defer rw.Unlock()
	return string(rw.buf)
}

type Status struct {
	PID       int
	Complete  bool
	LatestOut string
	LatestErr string
}

type Cmd struct {
	name string
	args []string
	ctx  context.Context
	env  []string
	dir  string
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
	return &Cmd{
		name:      name,
		args:      args,
		ctx:       nil,
		Mutex:     &sync.Mutex{},
		doneChan:  make(chan struct{}),
		stdoutBuf: NewOutputBuffer(),
		stderrBuf: NewOutputBuffer(),
	}
}

func NewCmdWithCtx(ctx context.Context, name string, args ...string) *Cmd {
	return &Cmd{
		name:      name,
		args:      args,
		ctx:       ctx,
		Mutex:     &sync.Mutex{},
		doneChan:  make(chan struct{}),
		stdoutBuf: NewOutputBuffer(),
		stderrBuf: NewOutputBuffer(),
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
	if c.statusChan != nil {
		return c.statusChan
	}
	c.statusChan = make(chan Status, 1)
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
				c.status.LatestOut = c.stdoutBuf.Latest()
				c.stdoutBuf = nil
			}
			if c.stderrBuf != nil {
				c.status.LatestErr = c.stderrBuf.Latest()
				c.stderrBuf = nil
			}
			c.final = true
		}
	} else {
		if c.stdoutBuf != nil {
			c.status.LatestOut = c.stdoutBuf.Latest()
		}
		if c.stderrBuf != nil {
			c.status.LatestErr = c.stderrBuf.Latest()
		}
	}
	return c.status
}

func (c *Cmd) run() {
	defer func() {
		c.statusChan <- c.Status()
		close(c.doneChan)
	}()

	var cmd *exec.Cmd
	if c.ctx != nil {
		cmd = exec.CommandContext(c.ctx, c.name, c.args...)
	} else {
		cmd = exec.Command(c.name, c.args...)
	}
	setProcessGroupID(cmd)
	switch {
	case c.stdoutBuf != nil && c.stderrBuf != nil:
		cmd.Stdout = c.stdoutBuf
		cmd.Stderr = c.stderrBuf
	case c.stdoutBuf != nil && c.stderrBuf == nil:
		cmd.Stdout = c.stdoutBuf
		cmd.Stderr = c.stdoutBuf
	default:
		cmd.Stdout = nil
		cmd.Stderr = nil
	}
	if len(c.env) > 0 {
		cmd.Env = c.env
	}
	if c.dir != "" {
		cmd.Dir = c.dir
	}
	if err := cmd.Start(); err != nil {
		c.Lock()
		c.done = true
		c.Unlock()
		return
	}
	c.Lock()
	c.status.PID = cmd.Process.Pid
	c.started = true
	c.Unlock()
	err := cmd.Wait()
	signaled := false
	if err != nil && fmt.Sprintf("%T", err) == "*exec.ExitError" {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		err = nil
		if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if waitStatus.Signaled() {
				signaled = true
				err = exitErr
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
