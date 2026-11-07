//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	conPTYWidth  = 120
	conPTYHeight = 30

	createUnicodeEnvironment   = 0x00000400
	createNewProcessGroup      = 0x00000200
	extendedStartupInfoPresent = 0x00080000
)

type conPTY struct {
	mu     sync.Mutex
	hpc    windows.Handle
	input  *os.File
	output *os.File
	closed bool
	width  int
	height int
}

func conPTYSupported() error {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	for _, name := range []string{"CreatePseudoConsole", "ClosePseudoConsole", "ResizePseudoConsole"} {
		if err := kernel32.NewProc(name).Find(); err != nil {
			return fmt.Errorf("Windows pseudo-console API %s is unavailable: %w", name, err)
		}
	}
	return nil
}

func newConPTY(width, height int) (*conPTY, error) {
	if err := conPTYSupported(); err != nil {
		return nil, err
	}
	if width <= 0 {
		width = conPTYWidth
	}
	if height <= 0 {
		height = conPTYHeight
	}
	c := &conPTY{width: width, height: height}
	var ptyInput, hostInput windows.Handle
	var hostOutput, ptyOutput windows.Handle
	if err := windows.CreatePipe(&ptyInput, &hostInput, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	if err := windows.CreatePipe(&hostOutput, &ptyOutput, nil, 0); err != nil {
		_ = windows.CloseHandle(ptyInput)
		_ = windows.CloseHandle(hostInput)
		return nil, fmt.Errorf("create ConPTY output pipe: %w", err)
	}
	if err := windows.SetHandleInformation(hostInput, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		closeWindowsHandles(ptyInput, hostInput, hostOutput, ptyOutput)
		return nil, fmt.Errorf("configure ConPTY input pipe: %w", err)
	}
	if err := windows.SetHandleInformation(hostOutput, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		closeWindowsHandles(ptyInput, hostInput, hostOutput, ptyOutput)
		return nil, fmt.Errorf("configure ConPTY output pipe: %w", err)
	}
	c.hpc = 0
	if err := windows.CreatePseudoConsole(
		windows.Coord{X: int16(width), Y: int16(height)},
		ptyInput,
		ptyOutput,
		0,
		&c.hpc,
	); err != nil {
		closeWindowsHandles(ptyInput, hostInput, hostOutput, ptyOutput)
		return nil, fmt.Errorf("create ConPTY: %w", err)
	}
	_ = windows.CloseHandle(ptyInput)
	_ = windows.CloseHandle(ptyOutput)
	c.input = os.NewFile(uintptr(hostInput), "portly-conpty-input")
	c.output = os.NewFile(uintptr(hostOutput), "portly-conpty-output")
	return c, nil
}

func closeWindowsHandles(handles ...windows.Handle) {
	for _, handle := range handles {
		if handle != 0 && handle != windows.InvalidHandle {
			_ = windows.CloseHandle(handle)
		}
	}
}

func (c *conPTY) Read(buffer []byte) (int, error) {
	c.mu.Lock()
	output := c.output
	closed := c.closed
	c.mu.Unlock()
	if closed || output == nil {
		return 0, os.ErrClosed
	}
	return output.Read(buffer)
}

func (c *conPTY) Write(data []byte) (int, error) {
	c.mu.Lock()
	input := c.input
	closed := c.closed
	c.mu.Unlock()
	if closed || input == nil {
		return 0, os.ErrClosed
	}
	return input.Write(data)
}

func (c *conPTY) Resize(width, height int) error {
	if c == nil {
		return nil
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("ConPTY dimensions must be positive")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.hpc == 0 {
		return os.ErrClosed
	}
	if err := windows.ResizePseudoConsole(c.hpc, windows.Coord{X: int16(width), Y: int16(height)}); err != nil {
		return err
	}
	c.width = width
	c.height = height
	return nil
}

func (c *conPTY) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	hpc, input, output := c.hpc, c.input, c.output
	c.hpc = 0
	c.input = nil
	c.output = nil
	c.mu.Unlock()
	if hpc != 0 {
		windows.ClosePseudoConsole(hpc)
	}
	if input != nil {
		_ = input.Close()
	}
	if output != nil {
		return output.Close()
	}
	return nil
}

func startChildConPTY(command, dir string, env []string) (*childProcess, io.ReadWriteCloser, error) {
	terminal, err := newConPTY(conPTYWidth, conPTYHeight)
	if err != nil {
		return nil, nil, err
	}
	shell, args := loginShell(command)
	commandLine := windows.ComposeCommandLine(append([]string{shell}, args...))
	commandLine16, err := windows.UTF16FromString(commandLine)
	if err != nil {
		_ = terminal.Close()
		return nil, nil, err
	}
	directory16, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		_ = terminal.Close()
		return nil, nil, err
	}
	environment := windowsEnvironmentBlock(env)
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		_ = terminal.Close()
		return nil, nil, fmt.Errorf("create process attribute list: %w", err)
	}
	defer attributes.Delete()
	if err := attachConPTYAttribute(attributes, terminal.hpc); err != nil {
		_ = terminal.Close()
		return nil, nil, fmt.Errorf("attach ConPTY to process: %w", err)
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput = 0
	startup.StdOutput = 0
	startup.StdErr = 0
	var process windows.ProcessInformation
	if err := windows.CreateProcess(
		nil,
		&commandLine16[0],
		nil,
		nil,
		false,
		createUnicodeEnvironment|createNewProcessGroup|extendedStartupInfoPresent,
		&environment[0],
		directory16,
		(*windows.StartupInfo)(unsafe.Pointer(&startup)),
		&process,
	); err != nil {
		_ = terminal.Close()
		return nil, nil, fmt.Errorf("start ConPTY process: %w", err)
	}
	_ = windows.CloseHandle(process.Thread)
	return &childProcess{
		pty:      terminal,
		pgid:     int(process.ProcessId),
		pidValue: int(process.ProcessId),
		waitFn: func() error {
			defer windows.CloseHandle(process.Process)
			if _, err := windows.WaitForSingleObject(process.Process, windows.INFINITE); err != nil {
				return err
			}
			var code uint32
			if err := windows.GetExitCodeProcess(process.Process, &code); err != nil {
				return err
			}
			return windowsExitError{code: int(code)}
		},
	}, terminal, nil
}

func attachConPTYAttribute(attributes *windows.ProcThreadAttributeListContainer, hpc windows.Handle) error {
	update := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")
	if err := update.Find(); err != nil {
		return err
	}
	result, _, lastErr := update.Call(
		uintptr(unsafe.Pointer(attributes.List())),
		0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		uintptr(hpc),
		unsafe.Sizeof(hpc),
		0,
		0,
	)
	if result != 0 {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("UpdateProcThreadAttribute returned failure")
}

func windowsEnvironmentBlock(values []string) []uint16 {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	block := make([]uint16, 0, len(copyValues)*2+1)
	for _, value := range copyValues {
		block = append(block, utf16.Encode([]rune(value))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	return block
}

var _ io.ReadWriteCloser = (*conPTY)(nil)
