package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type Config struct {
	Projects     []ProjectRef      `json:"projects"`
	WorktreeName map[string]string `json:"worktreeNames"`
	Window       WindowConfig      `json:"window"`
}

type ProjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type WindowConfig struct {
	AlwaysOnTop bool `json:"alwaysOnTop"`
	Configured  bool `json:"configured"`
}

type App struct {
	ctx      context.Context
	store    *FileStore
	scanner  *Scanner
	hooks    *HookBridge
	mu       sync.RWMutex
	dockMu   sync.Mutex
	state    *State
	scanning bool
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.store = NewFileStore(appDataDir())
	a.scanner = NewScanner(a.store)
	cfg, _ := a.store.Load()
	a.state = &State{
		Config:      cfg,
		Projects:    configProjectsOnly(cfg),
		LastUpdated: time.Now().Format(time.RFC3339),
	}
	a.hooks = NewHookBridge(a)
	wailsRuntime.WindowSetAlwaysOnTop(ctx, cfg.Window.AlwaysOnTop)
	go func() {
		time.Sleep(350 * time.Millisecond)
		a.PositionMonitorWindow()
	}()
}

func (a *App) shutdown(ctx context.Context) {
	if a.hooks != nil {
		_ = a.hooks.Stop(ctx)
	}
}

func (a *App) LoadState() (*State, error) {
	return a.CurrentState(), nil
}

func (a *App) CurrentState() *State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	state := cloneState(a.state, a.scanning)
	if a.hooks != nil {
		state.Hook = a.hooks.Status()
	}
	return state
}

func (a *App) RefreshState() (*State, error) {
	a.startRefresh()
	return a.CurrentState(), nil
}

func (a *App) AddProject(path string) (*State, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("project path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if !isDir(path) {
		return nil, errors.New("not a directory")
	}
	if _, err := a.scanner.AddProject(path); err != nil {
		return nil, err
	}
	a.reloadConfigOnly()
	a.startRefresh()
	return a.CurrentState(), nil
}

func (a *App) PickAndAddProject() (*State, error) {
	path, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择 Git 项目目录",
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return a.CurrentState(), nil
	}
	return a.AddProject(path)
}

func (a *App) RemoveProject(id string) (*State, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("project id is empty")
	}
	if _, err := a.scanner.RemoveProject(id); err != nil {
		return nil, err
	}
	a.reloadConfigOnly()
	a.startRefresh()
	return a.CurrentState(), nil
}

func (a *App) SetAlwaysOnTop(value bool) (*State, error) {
	_, err := a.scanner.UpdateWindowConfig(func(cfg WindowConfig) WindowConfig {
		cfg.AlwaysOnTop = value
		cfg.Configured = true
		return cfg
	})
	if err != nil {
		return nil, err
	}
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, value)
	a.reloadConfigOnly()
	return a.CurrentState(), nil
}

func (a *App) RenameWorktree(path, name string) (*State, error) {
	if _, err := a.scanner.RenameWorktree(path, name); err != nil {
		return nil, err
	}
	a.reloadConfigOnly()
	a.applyWorktreeNames()
	a.startRefresh()
	return a.CurrentState(), nil
}

func (a *App) Snapshot() (*State, error) {
	return a.CurrentState(), nil
}

func (a *App) PositionMonitorWindow() {
	if a.ctx == nil {
		return
	}
	a.dockMu.Lock()
	defer a.dockMu.Unlock()

	width := 390
	height := 620
	screenWidth := 1440
	screenHeight := 900
	if screens, err := wailsRuntime.ScreenGetAll(a.ctx); err == nil && len(screens) > 0 {
		screen := screens[0]
		for _, item := range screens {
			if item.IsCurrent {
				screen = item
				break
			}
		}
		if screen.Size.Width > 0 {
			screenWidth = screen.Size.Width
		} else if screen.Width > 0 {
			screenWidth = screen.Width
		}
		if screen.Size.Height > 0 {
			screenHeight = screen.Size.Height
		} else if screen.Height > 0 {
			screenHeight = screen.Height
		}
	}

	width = minInt(width, screenWidth-40)
	height = minInt(height, screenHeight-90)
	x := maxInt(0, screenWidth-width-8)
	y := maxInt(36, (screenHeight-height)/2)

	wailsRuntime.WindowSetSize(a.ctx, width, height)
	wailsRuntime.WindowSetPosition(a.ctx, x, y)
}

func (a *App) OpenTerminal(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("worktree path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("worktree path is not a directory")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Terminal", path).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", "cmd", "/K", "cd", "/d", path).Start()
	default:
		if terminal, err := exec.LookPath("x-terminal-emulator"); err == nil {
			return exec.Command(terminal, "--working-directory", path).Start()
		}
		if terminal, err := exec.LookPath("gnome-terminal"); err == nil {
			return exec.Command(terminal, "--working-directory", path).Start()
		}
		return errors.New("no supported terminal found")
	}
}

func (a *App) SetActivity(path, state, source string) (*State, error) {
	a.hooks.Set(path, Activity{
		State:     strings.TrimSpace(state),
		Source:    strings.TrimSpace(source),
		ChangedAt: time.Now().Format(time.RFC3339),
		EventPath: strings.TrimSpace(path),
	})
	a.applyActivitySnapshot()
	return a.CurrentState(), nil
}

func (a *App) startRefresh() {
	a.mu.Lock()
	if a.scanning {
		a.mu.Unlock()
		return
	}
	a.scanning = true
	state := cloneState(a.state, true)
	if a.hooks != nil {
		state.Hook = a.hooks.Status()
	}
	a.mu.Unlock()
	a.emitState(state)

	go func() {
		next, err := a.scanner.LoadState(a.hooks.Snapshot())
		a.mu.Lock()
		a.scanning = false
		if err == nil {
			a.state = next
		}
		state := cloneState(a.state, false)
		if a.hooks != nil {
			state.Hook = a.hooks.Status()
		}
		a.mu.Unlock()
		if err != nil {
			wailsRuntime.EventsEmit(a.ctx, "state:error", err.Error())
			return
		}
		a.emitState(state)
	}()
}

func (a *App) applyActivitySnapshot() {
	if a.hooks == nil {
		return
	}
	activity := a.hooks.Snapshot()
	a.mu.Lock()
	if a.state != nil {
		for pi := range a.state.Projects {
			for wi := range a.state.Projects[pi].Worktrees {
				path := a.state.Projects[pi].Worktrees[wi].Path
				a.state.Projects[pi].Worktrees[wi].Activity = activityForPath(path, activity)
			}
		}
	}
	state := cloneState(a.state, a.scanning)
	state.Hook = a.hooks.Status()
	a.mu.Unlock()
	a.emitState(state)
}

func (a *App) emitCurrentState() {
	a.emitState(a.CurrentState())
}

func (a *App) reloadConfigOnly() {
	cfg, err := a.store.Load()
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == nil {
		a.state = &State{}
	}
	a.state.Config = cfg
	a.state.Projects = syncProjectsToConfig(a.state.Projects, cfg)
}

func (a *App) emitState(state *State) {
	if a.ctx != nil && state != nil {
		wailsRuntime.EventsEmit(a.ctx, "state:updated", state)
	}
}

func appDataDir() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "worktree-float-go")
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func normalizeConfig(cfg Config) Config {
	if cfg.WorktreeName == nil {
		cfg.WorktreeName = map[string]string{}
	}
	if !cfg.Window.Configured {
		cfg.Window.AlwaysOnTop = true
	}
	sort.Slice(cfg.Projects, func(i, j int) bool {
		return cfg.Projects[i].Path < cfg.Projects[j].Path
	})
	return cfg
}

func configProjectsOnly(cfg Config) []ProjectVM {
	projects := make([]ProjectVM, 0, len(cfg.Projects))
	for _, ref := range cfg.Projects {
		projects = append(projects, ProjectVM{
			ID:   ref.ID,
			Name: firstNonEmpty(ref.Name, filepath.Base(ref.Path)),
			Root: ref.Path,
		})
	}
	return projects
}

func syncProjectsToConfig(existing []ProjectVM, cfg Config) []ProjectVM {
	byID := map[string]ProjectVM{}
	for _, project := range existing {
		byID[project.ID] = project
	}
	projects := make([]ProjectVM, 0, len(cfg.Projects))
	for _, ref := range cfg.Projects {
		if current, ok := byID[ref.ID]; ok {
			current.Name = firstNonEmpty(ref.Name, current.Name, filepath.Base(ref.Path))
			current.Root = firstNonEmpty(current.Root, ref.Path)
			projects = append(projects, current)
			continue
		}
		projects = append(projects, ProjectVM{
			ID:   ref.ID,
			Name: firstNonEmpty(ref.Name, filepath.Base(ref.Path)),
			Root: ref.Path,
		})
	}
	return projects
}

func (a *App) applyWorktreeNames() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == nil {
		return
	}
	names := a.state.Config.WorktreeName
	for pi := range a.state.Projects {
		for wi := range a.state.Projects[pi].Worktrees {
			worktree := &a.state.Projects[pi].Worktrees[wi]
			if displayName := strings.TrimSpace(names[worktree.Path]); displayName != "" {
				worktree.DisplayName = displayName
			} else {
				worktree.DisplayName = worktree.Name
			}
		}
		sort.Slice(a.state.Projects[pi].Worktrees, func(i, j int) bool {
			return a.state.Projects[pi].Worktrees[i].DisplayName < a.state.Projects[pi].Worktrees[j].DisplayName
		})
	}
}

func cloneState(state *State, scanning bool) *State {
	if state == nil {
		return &State{Scanning: scanning}
	}
	clone := *state
	clone.Scanning = scanning
	clone.Projects = append([]ProjectVM(nil), state.Projects...)
	for i := range clone.Projects {
		clone.Projects[i].Worktrees = append([]Worktree(nil), state.Projects[i].Worktrees...)
	}
	return &clone
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func decodeConfig(raw []byte) (Config, error) {
	var cfg Config
	if len(raw) == 0 {
		return normalizeConfig(cfg), nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	return normalizeConfig(cfg), nil
}
