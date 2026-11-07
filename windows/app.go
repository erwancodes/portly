//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

const defaultAPIPort = 7737

const releasesURL = "https://github.com/Melvynx/portly/releases"

type App struct {
	mu        sync.Mutex
	ctx       context.Context
	apiURL    string
	daemon    *exec.Cmd
	daemonLog *os.File
	tray      *systemTray
	quitOnce  sync.Once
}

func NewApp() *App {
	port := defaultAPIPort
	if raw := strings.TrimSpace(os.Getenv("PORTLY_API_PORT")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed < 65536 {
			port = parsed
		}
	}
	return &App{apiURL: "http://127.0.0.1:" + strconv.Itoa(port)}
}

func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	setWindowIcons()
	a.tray = newSystemTray(a)
	if err := a.tray.start(); err != nil {
		// The desktop shell remains usable without a tray on restricted desktop
		// sessions (for example Windows Server without Explorer).
		a.tray = nil
	}
	go func() { _ = a.ensureDaemon() }()
}

func (a *App) beforeClose(ctx context.Context) bool {
	// Closing the window is intentionally a hide operation. The supervisor and
	// its servers continue running until the explicit Quit action is selected.
	runtime.WindowHide(ctx)
	return true
}

func (a *App) shutdown(_ context.Context) {
	if a.tray != nil {
		a.tray.close()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.daemonLog != nil {
		_ = a.daemonLog.Close()
		a.daemonLog = nil
	}
}

func (a *App) showWindow() {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.WindowShow(ctx)
	}
}

type trayProject struct {
	Name string `json:"name"`
}

type trayStatus struct {
	Projects []trayProject `json:"projects"`
}

func (a *App) trayProjects() []trayProject {
	data, err := a.request(http.MethodGet, "/status", nil)
	if err != nil {
		return nil
	}
	var status trayStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil
	}
	return status.Projects
}

func (a *App) GetStatus() (string, error) {
	data, err := a.request(http.MethodGet, "/status", nil)
	return string(data), err
}

func (a *App) GetLogs(server string, tail int) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", errors.New("server is required")
	}
	if tail < 1 || tail > 5000 {
		tail = 200
	}
	path := "/logs?server=" + urlQueryEscape(server) + "&tail=" + strconv.Itoa(tail)
	data, err := a.request(http.MethodGet, path, nil)
	return string(data), err
}

func (a *App) WriteTerminal(server, data string) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", errors.New("server is required")
	}
	response, err := a.request(http.MethodPost, "/terminal/write", map[string]string{"server": server, "data": data})
	return string(response), err
}

func (a *App) ResizeTerminal(server string, width, height int) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", errors.New("server is required")
	}
	response, err := a.request(http.MethodPost, "/terminal/resize", map[string]any{"server": server, "width": width, "height": height})
	return string(response), err
}

func (a *App) Action(verb, target string) (string, error) {
	verb = strings.ToLower(strings.TrimSpace(verb))
	if verb != "start" && verb != "stop" && verb != "restart" {
		return "", errors.New("unsupported action")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("target is required")
	}
	body := map[string]string{}
	if strings.Contains(target, "/") {
		body["server"] = target
	} else {
		body["project"] = target
	}
	data, err := a.request(http.MethodPost, "/"+verb, body)
	return string(data), err
}

func (a *App) Quit() {
	a.quitOnce.Do(func() {
		_, _ = a.request(http.MethodPost, "/quit", map[string]any{})
		a.mu.Lock()
		ctx := a.ctx
		a.mu.Unlock()
		if ctx != nil {
			runtime.Quit(ctx)
		}
	})
}

func (a *App) OpenReleases() {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.BrowserOpenURL(ctx, releasesURL)
	}
}

func (a *App) request(method, path string, body any) ([]byte, error) {
	if err := a.ensureDaemon(); err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequest(method, a.apiURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Portly daemon unavailable: %w", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error *string         `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("invalid Portly response: %w", err)
	}
	if !envelope.OK {
		if envelope.Error != nil {
			return nil, errors.New(*envelope.Error)
		}
		return nil, errors.New("Portly request failed")
	}
	return envelope.Data, nil
}

func (a *App) ensureDaemon() error {
	if a.reachable() {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.reachable() {
		return nil
	}
	if a.daemon != nil && a.daemon.Process != nil {
		return fmt.Errorf("Portly daemon is starting")
	}
	executable, err := cliExecutable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(homeDir(), "logs"), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(homeDir(), "logs", "portly-gui.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	port := strings.TrimPrefix(a.apiURL, "http://127.0.0.1:")
	cmd := exec.Command(executable, "daemon", "--api-port", port)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("could not start Portly daemon: %w", err)
	}
	a.daemon = cmd
	a.daemonLog = logFile
	go a.watchDaemon(cmd, logFile)
	for attempt := 0; attempt < 50; attempt++ {
		if a.reachable() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("Portly daemon did not become ready")
}

func (a *App) watchDaemon(cmd *exec.Cmd, logFile *os.File) {
	_ = cmd.Wait()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.daemon != cmd {
		return
	}
	a.daemon = nil
	if a.daemonLog == logFile {
		_ = logFile.Close()
		a.daemonLog = nil
	}
}

func (a *App) reachable() bool {
	client := &http.Client{Timeout: 150 * time.Millisecond}
	resp, err := client.Get(a.apiURL + "/ping")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func cliExecutable() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("PORTLY_CLI_PATH")); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("PORTLY_CLI_PATH does not point to a file: %s", configured)
	}
	if executable, err := os.Executable(); err == nil {
		for _, candidate := range bundledCLICandidates(executable) {
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	if discovered, err := exec.LookPath("portly.exe"); err == nil {
		return discovered, nil
	}
	return "", errors.New("Portly.exe could not find portly.exe; set PORTLY_CLI_PATH")
}

func bundledCLICandidates(executable string) []string {
	dir := filepath.Dir(executable)
	candidates := []string{
		filepath.Join(dir, "bin", "portly.exe"),
		filepath.Join(dir, "cli", "portly.exe"),
		filepath.Join(dir, "portly.exe"),
	}
	filtered := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		// Windows paths are case-insensitive: the sibling `portly.exe`
		// candidate can resolve to the GUI itself (`Portly.exe`).
		if strings.EqualFold(filepath.Clean(candidate), filepath.Clean(executable)) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func homeDir() string {
	if override := strings.TrimSpace(os.Getenv("PORTLY_HOME")); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "portly")
}

func urlQueryEscape(value string) string {
	return strings.NewReplacer("%", "%25", " ", "%20", "/", "%2F", "?", "%3F", "#", "%23").Replace(value)
}
