// Package utils provides common utility functions for the file_parser package.
package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/izliang/bore/appctx"
	"github.com/izliang/bore/logit"
)

// cmdOption holds configuration options for the RunCmd function.
type cmdOption struct {
	in      io.ReadCloser // standard input reader
	out     io.Writer     // standard output writer
	err     io.Writer     // standard error writer
	timeout time.Duration // command execution timeout
}

// CmdOptionFunc is a function type used to configure RunCmd options using the functional options pattern.
type CmdOptionFunc func(*cmdOption)

// WithRunCmdStdin sets the standard input for the command.
func WithRunCmdStdin(in io.ReadCloser) CmdOptionFunc {
	return func(opt *cmdOption) {
		opt.in = in
	}
}

// WithRunCmdStdout sets a custom writer for standard output.
// Output will be written to both the custom writer and the internal buffer used for logging.
func WithRunCmdStdout(out io.Writer) CmdOptionFunc {
	return func(opt *cmdOption) {
		opt.out = out
	}
}

// WithRunCmdStderr sets a custom writer for standard error.
// Output will be written to both the custom writer and the internal buffer used for logging.
func WithRunCmdStderr(err io.Writer) CmdOptionFunc {
	return func(opt *cmdOption) {
		opt.err = err
	}
}

// WithRunCmdTimeout sets a custom timeout for command execution.
// If not specified, defaults to 5 minutes (300 seconds).
func WithRunCmdTimeout(timeout time.Duration) CmdOptionFunc {
	return func(opt *cmdOption) {
		opt.timeout = timeout
	}
}

// RunCmdSteam executes an external command with configurable options and timeout control.
// The command output (stdout and stderr) is always captured and logged at debug level via the appctx logger.
// If custom writers are provided via options, output will also be written to them.
//
// Parameters:
//
//	ctx  - Runtime instance that provides context and logging
//	name - The command name to execute
//	args - Command line arguments for the command
//	opts - Optional configuration options (WithStdin, WithStdout, WithStderr, WithTimeout)
//
// Returns:
//
//	error if the command fails, times out, or cannot be started; nil on success
func RunCmdSteam(ctx appctx.AppContext, name string, args []string, opts ...CmdOptionFunc) error {
	// Initialize options with defaults
	opt := &cmdOption{}
	for _, o := range opts {
		o(opt)
	}
	// Set default timeout to 5 minutes if not specified
	if opt.timeout == 0 {
		opt.timeout = 300 * time.Second
	}

	// Buffers to capture stdout and stderr for logging
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var err error
	// Log command output when function exits
	defer func() {
		if ctx.Logger() == nil {
			return
		}
		ctx.Logger().Info("run cmd steam", logit.StringField("name", name), logit.AnyField("args", args),
			logit.ErrorField("err", err),
		)
		ctx.Logger().Debug("run cmd steam", logit.StringField("name", name), logit.AnyField("args", args),
			logit.StringField("stdout", stdoutBuf.String(), logit.WithStrAllowNewline(true)),
			logit.StringField("stderr", stderrBuf.String(), logit.WithStrAllowNewline(true)),
			logit.ErrorField("err", err),
		)
	}()

	// Create context with timeout for the command
	nctx, cancel := context.WithTimeout(ctx.Context(), opt.timeout)
	defer cancel()

	// Create command with context
	cmd := exec.CommandContext(nctx, name, args...)

	// Configure stdin if provided
	if opt.in != nil {
		cmd.Stdin = opt.in
	}
	//defer func() {
	//	_ = opt.in.Close()
	//}()
	// Configure stdout - always capture to buffer for logging
	if opt.out != nil {
		cmd.Stdout = io.MultiWriter(opt.out, &stdoutBuf)
	} else {
		cmd.Stdout = &stdoutBuf
	}

	// Configure stderr - always capture to buffer for logging
	if opt.err != nil {
		cmd.Stderr = io.MultiWriter(opt.err, &stderrBuf)
	} else {
		cmd.Stderr = &stderrBuf
	}

	// Start the command
	if err = cmd.Start(); err != nil {
		return err
	}

	// Wait for command to complete
	err = cmd.Wait()
	// Check if context timeout exceeded
	if errors.Is(nctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("command timeout: %s", name)
	}
	return err
}

// RunCmd executes an external command synchronously with default timeout (5 minutes).
// It captures full stdout and stderr for logging, making it easily observable.
// Different from RunCmdSteam which supports streaming output to custom writers,
// RunCmd is for simple non-streaming execution with full output captured for logging.
//
// Parameters:
//
//	ctx  - Runtime instance that provides context and logging
//	name - The command name to execute
//	args - Command line arguments for the command
//
// Returns:
//
//	error if the command fails, times out, or cannot be started; nil on success
func RunCmd(ctx appctx.AppContext, name string, timeout time.Duration, args []string) error {

	// Buffers to capture stdout and stderr for logging
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var err error

	// Log command output when function exits
	defer func() {
		if ctx.Logger() == nil {
			return
		}
		ctx.Logger().Info("run cmd", logit.StringField("name", name), logit.AnyField("args", args),
			logit.ErrorField("err", err),
			logit.StringAbsField("stdout", stdoutBuf.String()),
			logit.StringAbsField("stderr", stderrBuf.String()),
		)
		ctx.Logger().Debug("run cmd", logit.StringField("name", name), logit.AnyField("args", args),
			logit.StringField("stdout", stdoutBuf.String(), logit.WithStrAllowNewline(true)),
			logit.StringField("stderr", stderrBuf.String(), logit.WithStrAllowNewline(true)),
			logit.ErrorField("err", err),
		)
	}()

	// Create context with timeout for the command
	nctx, cancel := context.WithTimeout(ctx.Context(), timeout)
	defer cancel()

	// Create command with context
	cmd := exec.CommandContext(nctx, name, args...)

	// Always capture output for logging
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Start and wait for command to complete
	err = cmd.Run()

	// Check if context timeout exceeded
	if errors.Is(nctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("command timeout: %s", name)
	}
	return err
}

type CmdOption struct {
	Timeout time.Duration

	// Whether to capture output (defaults to size-limited)
	CaptureOutput bool
	MaxOutputSize int64

	// Streaming output
	StdoutWriter io.Writer
	StderrWriter io.Writer

	// Whether to kill the entire process group
	KillGroup bool
}

func RunCMD(ctx appctx.AppContext, name string, args []string, opt CmdOption) error {
	var (
		stdoutBuf *LimitedBuffer
		stderrBuf *LimitedBuffer
		err       error
	)

	// Default limit 10MB to prevent excessive memory usage
	if opt.MaxOutputSize <= 0 {
		opt.MaxOutputSize = 10 * 1024 * 1024
	}

	nctx, cancel := context.WithTimeout(ctx.Context(), opt.Timeout)
	defer cancel()

	cmd := exec.CommandContext(nctx, name, args...)

	// Key point: process group (solves ffmpeg orphan process issue)
	if opt.KillGroup {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
	}

	// ---------- stdout ----------
	if opt.StdoutWriter != nil {
		cmd.Stdout = opt.StdoutWriter
	} else if opt.CaptureOutput {
		stdoutBuf = NewLimitedBuffer(opt.MaxOutputSize)
		cmd.Stdout = stdoutBuf
	} else {
		cmd.Stdout = io.Discard
	}

	// ---------- stderr ----------
	if opt.StderrWriter != nil {
		cmd.Stderr = opt.StderrWriter
	} else if opt.CaptureOutput {
		stderrBuf = NewLimitedBuffer(opt.MaxOutputSize)
		cmd.Stderr = stderrBuf
	} else {
		cmd.Stderr = io.Discard
	}

	// ---------- Execute ----------
	err = cmd.Start()
	if err != nil {
		return err
	}

	waitErr := cmd.Wait()

	// Timeout handling (kill entire process group)
	if errors.Is(nctx.Err(), context.DeadlineExceeded) {
		if opt.KillGroup && cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return fmt.Errorf("command timeout: %s", name)
	}
	return waitErr
}

type LimitedBuffer struct {
	buf bytes.Buffer
	max int64
}

func NewLimitedBuffer(max int64) *LimitedBuffer {
	return &LimitedBuffer{max: max}
}

func (l *LimitedBuffer) Write(p []byte) (int, error) {
	if int64(l.buf.Len()+len(p)) > l.max {
		remain := int(l.max) - l.buf.Len()
		if remain > 0 {
			l.buf.Write(p[:remain])
		}
		return len(p), nil // discard remaining data
	}
	return l.buf.Write(p)
}

func (l *LimitedBuffer) Bytes() []byte {
	return l.buf.Bytes()
}
