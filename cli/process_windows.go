//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Windows does not expose POSIX process groups. A new process group plus
// taskkill's /T option gives Portly a predictable tree boundary for the MVP.
// ConPTY is attempted first; pipes remain the compatibility fallback for
// older Windows builds where the pseudo-console API is unavailable.
type childProcess struct {
	cmd      *exec.Cmd
	pty      io.ReadWriteCloser
	input    io.WriteCloser
	pgid     int
	pidValue int
	waitFn   func() error
}

func loginShell(command string) (string, []string) {
	if configured := os.Getenv("PORTLY_SHELL"); configured != "" {
		if shell, err := exec.LookPath(configured); err == nil {
			return windowsShellArgs(shell, command)
		}
	}
	for _, candidate := range []string{"pwsh.exe", "powershell.exe", "cmd.exe"} {
		if shell, err := exec.LookPath(candidate); err == nil {
			return windowsShellArgs(shell, command)
		}
	}
	return "cmd.exe", []string{"/D", "/S", "/C", command}
}

func windowsShellArgs(shell, command string) (string, []string) {
	name := strings.ToLower(filepath.Base(shell))
	if name == "pwsh.exe" || name == "pwsh" || name == "powershell.exe" || name == "powershell" {
		return shell, []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command}
	}
	return shell, []string{"/D", "/S", "/C", command}
}

func startChild(command, dir string, env []string) (*childProcess, io.Reader, error) {
	if child, reader, err := startChildConPTY(command, dir, env); err == nil {
		return child, reader, nil
	}
	return startChildPiped(command, dir, env)
}

func newShellCommand(command, dir string, env []string) *exec.Cmd {
	shell, args := loginShell(command)
	cmd := exec.Command(shell, args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd
}

func startChildPiped(command, dir string, env []string) (*childProcess, io.Reader, error) {
	cmd := newShellCommand(command, dir, env)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		_ = input.Close()
		return nil, nil, err
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, err
	}
	if cmd.Process == nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, fmt.Errorf("process did not start")
	}
	_ = writer.Close()
	return &childProcess{cmd: cmd, input: input, pgid: cmd.Process.Pid}, reader, nil
}

func (c *childProcess) write(data []byte) (int, error) {
	if c == nil {
		return 0, fmt.Errorf("process is not running")
	}
	if c.pty != nil {
		return c.pty.Write(data)
	}
	if c.input != nil {
		return c.input.Write(data)
	}
	return 0, fmt.Errorf("process has no writable terminal")
}

func (c *childProcess) resize(width, height int) error {
	if c == nil || c.pty == nil {
		return fmt.Errorf("process does not expose a resizable terminal")
	}
	resizer, ok := c.pty.(interface{ Resize(int, int) error })
	if !ok {
		return fmt.Errorf("process does not expose a resizable terminal")
	}
	return resizer.Resize(width, height)
}

func (c *childProcess) terminate() {
	if c != nil {
		_ = terminatePID(c.pgid)
	}
}

func (c *childProcess) forceKill() {
	if c != nil {
		forceKillProcess(c.pgid)
	}
}

func terminatePID(pid int) error {
	if pid <= 0 {
		return nil
	}
	cmd := exec.Command("taskkill.exe", "/PID", fmt.Sprintf("%d", pid), "/T")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func forceKillProcess(pid int) {
	if pid <= 0 {
		return
	}
	_ = forceTerminatePID(pid)
}

func forceTerminatePID(pid int) error {
	if pid <= 0 {
		return nil
	}
	cmd := exec.Command("taskkill.exe", "/PID", fmt.Sprintf("%d", pid), "/T", "/F")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill /F: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *childProcess) wait() error {
	if c == nil {
		return nil
	}
	var err error
	if c.waitFn != nil {
		err = c.waitFn()
	} else if c.cmd != nil {
		err = c.cmd.Wait()
	}
	if c.pty != nil {
		_ = c.pty.Close()
	}
	if c.input != nil {
		_ = c.input.Close()
	}
	return err
}

func (c *childProcess) pid() int {
	if c == nil {
		return 0
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Pid
	}
	return c.pidValue
}

func exitCodeFromWait(err error) int {
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode()
	}
	if exit, ok := err.(interface{ ExitCode() int }); ok {
		return exit.ExitCode()
	}
	return 1
}

type windowsExitError struct{ code int }

func (e windowsExitError) Error() string { return fmt.Sprintf("process exited with code %d", e.code) }
func (e windowsExitError) ExitCode() int { return e.code }

func childEnv(base map[string]string, server string, port *int, extra map[string]string) []string {
	envMap := map[string]string{}
	for _, kv := range os.Environ() {
		if i := indexByte([]byte(kv), '='); i > 0 {
			envMap[kv[:i]] = kv[i+1:]
		}
	}
	delete(envMap, "NO_COLOR")
	envMap["TERM"] = "xterm-256color"
	envMap["COLORTERM"] = "truecolor"
	envMap["FORCE_COLOR"] = "1"
	envMap["CLICOLOR"] = "1"
	envMap["CLICOLOR_FORCE"] = "1"
	envMap["TERM_PROGRAM"] = "Portly"
	envMap["PORTLY"] = "1"
	envMap["PORTLY_SERVER"] = server
	if port != nil {
		envMap["PORT"] = fmt.Sprintf("%d", *port)
	}
	for k, v := range extra {
		envMap[k] = v
	}
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	return out
}
