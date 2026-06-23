package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const hookBridgePort = 48731
const finishedVisibleFor = 4200 * time.Millisecond

var workingHookEvents = map[string]bool{
	"UserPromptSubmit": true,
	"PreToolUse":       true,
	"PostToolUse":      true,
	"SubagentStart":    true,
	"SessionStart":     true,
}

var finishedHookEvents = map[string]bool{
	"Stop":          true,
	"SubagentStop":  true,
	"TaskCompleted": true,
	"SessionEnd":    true,
	"Notification":  true,
}

var errorHookEvents = map[string]bool{
	"StopFailure": true,
}

type HookBridge struct {
	app       *App
	mu        sync.RWMutex
	state     map[string]Activity
	sessions  map[string]map[string]bool
	timers    map[string]*time.Timer
	server    *http.Server
	status    HookStatus
	pathRegex *regexp.Regexp
}

type hookPayload struct {
	Provider string         `json:"provider"`
	Raw      map[string]any `json:"raw"`
}

type normalizedHookEvent struct {
	Provider  string
	EventName string
	CWD       string
	Raw       map[string]any
}

func NewHookBridge(app *App) *HookBridge {
	h := &HookBridge{
		app:       app,
		state:     map[string]Activity{},
		sessions:  map[string]map[string]bool{},
		timers:    map[string]*time.Timer{},
		status:    HookStatus{Port: hookBridgePort},
		pathRegex: regexp.MustCompile(`/Users/[^\s"'` + "`" + `]+`),
	}
	h.start()
	return h
}

func (h *HookBridge) Set(path string, activity Activity) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.setLocked(path, activity)
}

func (h *HookBridge) Snapshot() map[string]Activity {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]Activity, len(h.state))
	for k, v := range h.state {
		out[k] = v
	}
	return out
}

func (h *HookBridge) Status() HookStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

func (h *HookBridge) start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/activity", h.handleActivity)
	server := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", hookBridgePort),
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}
	h.server = server

	go func() {
		listener, err := net.Listen("tcp", server.Addr)
		h.mu.Lock()
		if err != nil {
			h.status = HookStatus{Enabled: false, Port: hookBridgePort, Error: err.Error()}
			h.mu.Unlock()
			h.app.emitCurrentState()
			return
		}
		h.status = HookStatus{Enabled: true, Port: hookBridgePort}
		h.mu.Unlock()
		h.app.emitCurrentState()

		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			h.mu.Lock()
			h.status = HookStatus{Enabled: false, Port: hookBridgePort, Error: err.Error()}
			h.mu.Unlock()
			h.app.emitCurrentState()
		}
	}()
}

func (h *HookBridge) Stop(ctx context.Context) error {
	if h.server == nil {
		return nil
	}
	return h.server.Shutdown(ctx)
}

func (h *HookBridge) handleHealth(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeJSON(response, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	writeJSON(response, http.StatusOK, map[string]any{
		"ok":     true,
		"status": h.status,
		"active": h.state,
	})
}

func (h *HookBridge) handleActivity(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeJSON(response, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	event, err := h.readHookEvent(request)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	candidates := h.extractCandidatePaths(event.Raw)
	if event.CWD != "" {
		candidates = append([]string{event.CWD}, candidates...)
	}
	worktreePath := h.resolveWorktree(candidates)
	if worktreePath == "" {
		writeJSON(response, http.StatusAccepted, map[string]any{
			"ok":      true,
			"mapped":  false,
			"message": "No matching worktree is currently shown.",
		})
		return
	}
	h.applyHookEvent(worktreePath, event)
	writeJSON(response, http.StatusOK, map[string]any{
		"ok":           true,
		"mapped":       true,
		"worktreePath": worktreePath,
	})
}

func (h *HookBridge) readHookEvent(request *http.Request) (normalizedHookEvent, error) {
	defer request.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(request.Body, 1024*1024))
	if err != nil {
		return normalizedHookEvent{}, err
	}
	var payload hookPayload
	if len(rawBody) > 0 {
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			return normalizedHookEvent{}, fmt.Errorf("invalid JSON: %w", err)
		}
	}
	raw := payload.Raw
	if raw == nil {
		raw = map[string]any{}
		if len(rawBody) > 0 {
			_ = json.Unmarshal(rawBody, &raw)
		}
	}
	queryProvider := ""
	if parsed, err := url.Parse(request.URL.String()); err == nil {
		queryProvider = parsed.Query().Get("provider")
	}
	provider := firstNonEmpty(payload.Provider, stringFrom(raw["provider"]), queryProvider, "hook")
	eventName := firstNonEmpty(
		stringFrom(raw["hook_event_name"]),
		stringFrom(raw["hookEventName"]),
		stringFrom(raw["event"]),
		stringFrom(raw["eventName"]),
		"Unknown",
	)
	cwd := firstNonEmpty(
		stringFrom(raw["cwd"]),
		stringFrom(nested(raw, "workspace", "current_dir")),
		stringFrom(nested(raw, "workspace", "project_dir")),
		stringFrom(nested(raw, "workspace", "git_worktree")),
		stringFrom(nested(raw, "worktree", "path")),
	)
	return normalizedHookEvent{Provider: provider, EventName: eventName, CWD: cwd, Raw: raw}, nil
}

func (h *HookBridge) extractCandidatePaths(raw map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(value any, depth int)
	walk = func(value any, depth int) {
		if depth > 7 || value == nil {
			return
		}
		switch typed := value.(type) {
		case string:
			h.addCandidate(&out, seen, typed)
			for _, match := range h.pathRegex.FindAllString(typed, -1) {
				h.addCandidate(&out, seen, match)
			}
		case []any:
			for _, item := range typed {
				walk(item, depth+1)
			}
		case map[string]any:
			for key, item := range typed {
				if isPathLikeKey(key) {
					h.addCandidate(&out, seen, stringFrom(item))
				}
				walk(item, depth+1)
			}
		}
	}
	walk(raw, 0)
	return out
}

func (h *HookBridge) addCandidate(out *[]string, seen map[string]bool, value string) {
	if value == "" || !strings.HasPrefix(value, "/") {
		return
	}
	clean := filepath.Clean(value)
	if !seen[clean] {
		seen[clean] = true
		*out = append(*out, clean)
	}
}

func (h *HookBridge) resolveWorktree(candidates []string) string {
	state := h.app.CurrentState()
	var worktreePaths []string
	for _, project := range state.Projects {
		for _, worktree := range project.Worktrees {
			worktreePaths = append(worktreePaths, filepath.Clean(worktree.Path))
		}
	}
	sortStringsByLengthDesc(worktreePaths)
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		for _, worktreePath := range worktreePaths {
			if candidate == worktreePath || strings.HasPrefix(candidate, worktreePath+string(os.PathSeparator)) {
				return worktreePath
			}
		}
	}
	for _, candidate := range candidates {
		root, err := findGitRoot(candidate)
		if err != nil {
			continue
		}
		for _, worktreePath := range worktreePaths {
			if root == worktreePath || strings.HasPrefix(root, worktreePath+string(os.PathSeparator)) {
				return worktreePath
			}
		}
	}
	return ""
}

func (h *HookBridge) applyHookEvent(worktreePath string, event normalizedHookEvent) {
	sessionKey := firstNonEmpty(
		stringFrom(event.Raw["session_id"]),
		stringFrom(event.Raw["sessionId"]),
		stringFrom(event.Raw["transcript_path"]),
		stringFrom(event.Raw["conversation_id"]),
		"default",
	)
	now := time.Now().Format(time.RFC3339)
	activity := Activity{
		State:     "working",
		Source:    event.Provider,
		ChangedAt: now,
		EventPath: worktreePath,
		EventName: event.EventName,
	}

	h.mu.Lock()
	if timer := h.timers[worktreePath]; timer != nil {
		timer.Stop()
		delete(h.timers, worktreePath)
	}
	if errorHookEvents[event.EventName] {
		activity.State = "error"
		h.setLocked(worktreePath, activity)
	} else if finishedHookEvents[event.EventName] {
		h.removeSessionLocked(worktreePath, sessionKey)
		if h.hasSessionsLocked(worktreePath) {
			activity.State = "working"
			h.setLocked(worktreePath, activity)
		} else {
			activity.State = "finished"
			h.setLocked(worktreePath, activity)
			h.timers[worktreePath] = time.AfterFunc(finishedVisibleFor, func() {
				h.Set(worktreePath, Activity{State: "idle"})
				h.app.applyActivitySnapshot()
			})
		}
	} else if workingHookEvents[event.EventName] || event.EventName != "" {
		h.addSessionLocked(worktreePath, sessionKey)
		h.setLocked(worktreePath, activity)
	}
	h.mu.Unlock()
	h.app.applyActivitySnapshot()
}

func (h *HookBridge) setLocked(path string, activity Activity) {
	if path == "" {
		return
	}
	if activity.State == "" || activity.State == "idle" || activity.State == "done" {
		delete(h.state, path)
		return
	}
	h.state[path] = activity
}

func (h *HookBridge) addSessionLocked(worktreePath, sessionKey string) {
	sessions := h.sessions[worktreePath]
	if sessions == nil {
		sessions = map[string]bool{}
		h.sessions[worktreePath] = sessions
	}
	sessions[sessionKey] = true
}

func (h *HookBridge) removeSessionLocked(worktreePath, sessionKey string) {
	sessions := h.sessions[worktreePath]
	if sessions == nil {
		return
	}
	delete(sessions, sessionKey)
	if len(sessions) == 0 {
		delete(h.sessions, worktreePath)
	}
}

func (h *HookBridge) hasSessionsLocked(worktreePath string) bool {
	return len(h.sessions[worktreePath]) > 0
}

func writeJSON(response http.ResponseWriter, statusCode int, payload any) {
	response.Header().Set("content-type", "application/json")
	response.WriteHeader(statusCode)
	_ = json.NewEncoder(response).Encode(payload)
}

func stringFrom(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func nested(raw map[string]any, keys ...string) any {
	var current any = raw
	for _, key := range keys {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = asMap[key]
	}
	return current
}

func isPathLikeKey(key string) bool {
	switch strings.ToLower(key) {
	case "cwd", "workdir", "path", "file", "file_path", "filename", "target", "directory":
		return true
	default:
		return false
	}
}

func sortStringsByLengthDesc(values []string) {
	for i := 0; i < len(values)-1; i++ {
		for j := i + 1; j < len(values); j++ {
			if len(values[j]) > len(values[i]) {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
