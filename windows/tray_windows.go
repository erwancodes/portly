//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy        = 0x0002
	wmClose          = 0x0010
	wmCommand        = 0x0111
	wmApp            = 0x8000
	wmLButtonDblClk  = 0x0203
	wmLButtonUp      = 0x0202
	wmRButtonUp      = 0x0205
	wmContextMenu    = 0x007B
	trayCallback     = wmApp + 1
	nimAdd           = 0x00000000
	nimDelete        = 0x00000002
	nifMessage       = 0x00000001
	nifIcon          = 0x00000002
	nifTip           = 0x00000004
	mfString         = 0x00000000
	mfSeparator      = 0x00000800
	mfPopup          = 0x00000010
	tpmRightButton   = 0x00000002
	tpmReturnCommand = 0x00000100
	idiApplication   = 32512
	trayOpen         = 1001
	trayDashboard    = 1002
	trayQuit         = 1003
	trayProjectBase  = 1100
)

var (
	user32Tray  = windows.NewLazySystemDLL("user32.dll")
	shell32Tray = windows.NewLazySystemDLL("shell32.dll")

	procCreateWindowExW  = user32Tray.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32Tray.NewProc("DefWindowProcW")
	procDestroyWindow    = user32Tray.NewProc("DestroyWindow")
	procDispatchMessageW = user32Tray.NewProc("DispatchMessageW")
	procGetCursorPos     = user32Tray.NewProc("GetCursorPos")
	procGetMessageW      = user32Tray.NewProc("GetMessageW")
	procLoadIconW        = user32Tray.NewProc("LoadIconW")
	procLoadImageW       = user32Tray.NewProc("LoadImageW")
	procGetSystemMetrics = user32Tray.NewProc("GetSystemMetrics")
	procPostMessageW     = user32Tray.NewProc("PostMessageW")
	procPostQuitMessage  = user32Tray.NewProc("PostQuitMessage")
	procRegisterClassExW = user32Tray.NewProc("RegisterClassExW")
	procSetForeground    = user32Tray.NewProc("SetForegroundWindow")
	procTrackPopupMenu   = user32Tray.NewProc("TrackPopupMenu")
	procTranslateMessage = user32Tray.NewProc("TranslateMessage")
	procCreatePopupMenu  = user32Tray.NewProc("CreatePopupMenu")
	procAppendMenuW      = user32Tray.NewProc("AppendMenuW")
	procDestroyMenu      = user32Tray.NewProc("DestroyMenu")
	procShellNotifyIconW = shell32Tray.NewProc("Shell_NotifyIconW")
)

type trayPoint struct {
	x int32
	y int32
}

type trayMessage struct {
	hWnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      trayPoint
	private uint32
}

type trayWindowClass struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guid             [16]byte
	hBalloonIcon     uintptr
}

type systemTray struct {
	app     *App
	done    chan struct{}
	ready   chan error
	readyMu sync.Once
	doneMu  sync.Once
	closeMu sync.Once
	hWnd    uintptr
}

func newSystemTray(app *App) *systemTray {
	return &systemTray{app: app, done: make(chan struct{}), ready: make(chan error, 1)}
}

func (t *systemTray) start() error {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := t.run(); err != nil {
			t.readyMu.Do(func() { t.ready <- err })
		}
	}()
	return <-t.ready
}

func (t *systemTray) close() {
	t.closeMu.Do(func() {
		if t.hWnd != 0 {
			_, _, _ = procPostMessageW.Call(t.hWnd, wmClose, 0, 0)
		}
		<-t.done
	})
}

func (t *systemTray) finish() {
	t.doneMu.Do(func() { close(t.done) })
}

func (t *systemTray) run() error {
	defer t.finish()
	className, err := windows.UTF16PtrFromString("PortlyTrayWindow")
	if err != nil {
		return err
	}
	wndProc := windows.NewCallback(t.windowProc)
	icon := appIcon()
	class := trayWindowClass{
		cbSize:        uint32(unsafe.Sizeof(trayWindowClass{})),
		lpfnWndProc:   wndProc,
		hIcon:         icon,
		hIconSm:       icon,
		lpszClassName: className,
	}
	if registered, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); registered == 0 && callErr != syscall.Errno(1410) {
		return fmt.Errorf("register tray window: %w", callErr)
	}
	hWnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0,
		0, 0, 0, 0,
		0, 0, 0, 0,
	)
	if hWnd == 0 {
		return fmt.Errorf("create tray window: %w", callErr)
	}
	t.hWnd = hWnd
	if err := t.addIcon(icon); err != nil {
		_, _, _ = procDestroyWindow.Call(hWnd)
		return err
	}

	t.readyMu.Do(func() { t.ready <- nil })
	for {
		var message trayMessage
		result, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("tray message loop: %w", getErr)
		}
		if result == 0 {
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
	return nil
}

func (t *systemTray) addIcon(icon uintptr) error {
	tip, err := windows.UTF16FromString("Portly")
	if err != nil {
		return err
	}
	var data notifyIconData
	data.cbSize = uint32(unsafe.Sizeof(data))
	data.hWnd = t.hWnd
	data.uID = 1
	data.uFlags = nifMessage | nifIcon | nifTip
	data.uCallbackMessage = trayCallback
	data.hIcon = icon
	copy(data.szTip[:], tip)
	added, _, callErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&data)))
	if added == 0 {
		return fmt.Errorf("add Portly tray icon: %w", callErr)
	}
	return nil
}

func (t *systemTray) removeIcon() {
	var data notifyIconData
	data.cbSize = uint32(unsafe.Sizeof(data))
	data.hWnd = t.hWnd
	data.uID = 1
	_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
}

func (t *systemTray) windowProc(hWnd uintptr, message uint32, wParam uintptr, lParam uintptr) uintptr {
	switch message {
	case trayCallback:
		switch uint32(lParam) {
		case wmLButtonDblClk, wmLButtonUp:
			t.app.showWindow()
		case wmRButtonUp, wmContextMenu:
			t.showMenu(hWnd)
		}
	case wmCommand:
		// Commands are handled synchronously by TrackPopupMenu using
		// TPM_RETURNCMD, so this branch only keeps the hidden window sane.
		return 0
	case wmDestroy:
		t.removeIcon()
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	case wmClose:
		_, _, _ = procDestroyWindow.Call(hWnd)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hWnd, uintptr(message), wParam, lParam)
	return result
}

func (t *systemTray) showMenu(hWnd uintptr) {
	menuHandle, _, _ := procCreatePopupMenu.Call()
	if menuHandle == 0 {
		return
	}
	defer procDestroyMenu.Call(menuHandle)
	appendMenuText(menuHandle, trayOpen, "Open Portly")
	appendMenuText(menuHandle, trayDashboard, "Open dashboard")
	appendMenuText(menuHandle, 0, "")

	actions := map[uint32]func(){
		trayOpen:      t.app.showWindow,
		trayDashboard: t.app.showWindow,
		trayQuit:      t.app.Quit,
	}
	for index, project := range t.app.trayProjects() {
		projectMenu, _, _ := procCreatePopupMenu.Call()
		if projectMenu == 0 {
			continue
		}
		startID := uint32(trayProjectBase + index*2)
		stopID := startID + 1
		appendMenuText(projectMenu, startID, "Start")
		appendMenuText(projectMenu, stopID, "Stop")
		appendMenuPopup(menuHandle, projectMenu, project.Name)
		actions[startID] = func() { _, _ = t.app.Action("start", project.Name) }
		actions[stopID] = func() { _, _ = t.app.Action("stop", project.Name) }
	}
	appendMenuText(menuHandle, 0, "")
	appendMenuText(menuHandle, trayQuit, "Quit Portly")

	var point trayPoint
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	_, _, _ = procSetForeground.Call(hWnd)
	command, _, _ := procTrackPopupMenu.Call(menuHandle, tpmReturnCommand|tpmRightButton, uintptr(point.x), uintptr(point.y), 0, hWnd, 0)
	if action := actions[uint32(command)]; action != nil {
		action()
	}
}

func appendMenuText(menuHandle uintptr, id uint32, label string) {
	if id == 0 {
		_, _, _ = procAppendMenuW.Call(menuHandle, mfSeparator, 0, 0)
		return
	}
	text, _ := windows.UTF16PtrFromString(label)
	_, _, _ = procAppendMenuW.Call(menuHandle, mfString, uintptr(id), uintptr(unsafe.Pointer(text)))
}

func appendMenuPopup(menuHandle, popup uintptr, label string) {
	text, _ := windows.UTF16PtrFromString(label)
	_, _, _ = procAppendMenuW.Call(menuHandle, mfPopup, popup, uintptr(unsafe.Pointer(text)))
}

// appIcon loads the icon Wails embeds in the executable (resource ID 3) at the
// small-icon size, falling back to the stock application icon.
func appIcon() uintptr {
	const imageIcon, smCxSmIcon, smCySmIcon, appIconID = 1, 49, 50, 3
	var module windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &module); err == nil {
		cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
		cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
		if icon, _, _ := procLoadImageW.Call(uintptr(module), appIconID, imageIcon, cx, cy, 0); icon != 0 {
			return icon
		}
	}
	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	return icon
}

var (
	procEnumWindows              = user32Tray.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32Tray.NewProc("GetWindowThreadProcessId")
	procGetClassNameW            = user32Tray.NewProc("GetClassNameW")
	procSendMessageW             = user32Tray.NewProc("SendMessageW")
)

// setWindowIcons gives the Wails window both icon sizes. Wails only sets the
// small one, so the taskbar fell back to the executable's icon, which Windows
// caches by path and keeps showing stale after the icon changes.
func setWindowIcons() {
	const wmSetIcon, iconSmall, iconBig, imageIcon, appIconID = 0x0080, 0, 1, 1, 3
	const smCxIcon, smCyIcon, smCxSmIcon, smCySmIcon = 11, 12, 49, 50
	var module windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &module); err != nil {
		return
	}
	load := func(cxMetric, cyMetric uintptr) uintptr {
		cx, _, _ := procGetSystemMetrics.Call(cxMetric)
		cy, _, _ := procGetSystemMetrics.Call(cyMetric)
		icon, _, _ := procLoadImageW.Call(uintptr(module), appIconID, imageIcon, cx, cy, 0)
		return icon
	}
	big, small := load(smCxIcon, smCyIcon), load(smCxSmIcon, smCySmIcon)
	pid := uint32(windows.GetCurrentProcessId())
	callback := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		name := make([]uint16, 64)
		n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if windows.UTF16ToString(name[:n]) != "wailsWindow" {
			return 1
		}
		if big != 0 {
			_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconBig, big)
		}
		if small != 0 {
			_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, small)
		}
		return 1
	})
	_, _, _ = procEnumWindows.Call(callback, 0)
}
