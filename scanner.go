package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Scanner struct {
	store *FileStore
	mu    sync.Mutex
}

func NewScanner(store *FileStore) *Scanner {
	return &Scanner{store: store}
}

func (s *Scanner) LoadState(activity map[string]Activity) (*State, error) {
	cfg, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	projects := make([]ProjectVM, 0, len(cfg.Projects))
	for _, ref := range cfg.Projects {
		project, _ := s.inspectProject(ref, cfg, activity)
		projects = append(projects, project)
	}
	return &State{
		Config:      cfg,
		Projects:    projects,
		LastUpdated: time.Now().Format(time.RFC3339),
	}, nil
}

func (s *Scanner) AddProject(path string) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.store.Load()
	if err != nil {
		return Config{}, err
	}
	root, err := findGitRoot(path)
	if err != nil {
		return Config{}, err
	}
	project, err := projectRefFromRoot(root)
	if err != nil {
		return Config{}, err
	}
	found := false
	next := make([]ProjectRef, 0, len(cfg.Projects)+1)
	for _, existing := range cfg.Projects {
		if existing.ID == project.ID {
			found = true
			next = append(next, project)
			continue
		}
		next = append(next, existing)
	}
	if !found {
		next = append(next, project)
	}
	cfg.Projects = next
	return s.store.Save(cfg)
}

func (s *Scanner) RemoveProject(id string) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.store.Load()
	if err != nil {
		return Config{}, err
	}
	next := make([]ProjectRef, 0, len(cfg.Projects))
	for _, project := range cfg.Projects {
		if project.ID != id {
			next = append(next, project)
		}
	}
	cfg.Projects = next
	return s.store.Save(cfg)
}

func (s *Scanner) RenameWorktree(path, name string) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.store.Load()
	if err != nil {
		return Config{}, err
	}
	if cfg.WorktreeName == nil {
		cfg.WorktreeName = map[string]string{}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		delete(cfg.WorktreeName, path)
	} else {
		cfg.WorktreeName[path] = name
	}
	return s.store.Save(cfg)
}

func (s *Scanner) UpdateWindowConfig(mutator func(WindowConfig) WindowConfig) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.store.Load()
	if err != nil {
		return Config{}, err
	}
	cfg.Window = mutator(cfg.Window)
	return s.store.Save(cfg)
}

func (s *Scanner) inspectProject(ref ProjectRef, cfg Config, activity map[string]Activity) (ProjectVM, error) {
	root, err := findGitRoot(ref.Path)
	if err != nil {
		return ProjectVM{ID: ref.ID, Name: ref.Name, Root: ref.Path, Error: err.Error()}, nil
	}
	commonDir, err := gitCommonDir(root)
	if err != nil {
		return ProjectVM{ID: ref.ID, Name: ref.Name, Root: root, Error: err.Error()}, nil
	}
	worktreesRaw, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return ProjectVM{ID: ref.ID, Name: ref.Name, Root: root, Error: err.Error()}, nil
	}
	worktrees := parseWorktrees(worktreesRaw)
	for i := range worktrees {
		worktrees[i].DisplayName = cfg.WorktreeName[worktrees[i].Path]
		if worktrees[i].DisplayName == "" {
			worktrees[i].DisplayName = worktrees[i].Name
		}
		worktrees[i].Status = gitStateForPath(worktrees[i].Path)
		worktrees[i].Activity = activityForPath(worktrees[i].Path, activity)
	}
	sort.Slice(worktrees, func(i, j int) bool {
		return worktrees[i].DisplayName < worktrees[j].DisplayName
	})
	name := ref.Name
	if name == "" {
		name = filepath.Base(root)
	}
	return ProjectVM{
		ID:        commonDir,
		Name:      name,
		Root:      root,
		Worktrees: worktrees,
	}, nil
}

func projectRefFromRoot(root string) (ProjectRef, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ProjectRef{}, err
	}
	commonDir, err := gitCommonDir(root)
	if err != nil {
		return ProjectRef{}, err
	}
	return ProjectRef{
		ID:   commonDir,
		Name: filepath.Base(root),
		Path: root,
	}, nil
}

func gitCommonDir(root string) (string, error) {
	raw, err := git(root, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Clean(filepath.Join(root, raw)), nil
}

func git(cwd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", cwd}, args...)...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git command timed out in %s", cwd)
		}
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return strings.TrimSpace(out.String()), nil
}

func findGitRoot(start string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(start))
	if err != nil {
		return "", err
	}
	current := filepath.Clean(abs)
	for {
		if isGitRepository(current) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("请选择 Git 项目目录或其子目录")
		}
		current = parent
	}
}

func isGitRepository(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	gitPath := filepath.Join(dir, ".git")
	gitInfo, err := os.Lstat(gitPath)
	if err != nil {
		return false
	}
	return gitInfo.IsDir() || gitInfo.Mode().IsRegular()
}

func parseWorktrees(raw string) []Worktree {
	var current *Worktree
	worktrees := []Worktree{}
	flush := func() {
		if current != nil && current.Path != "" {
			worktrees = append(worktrees, *current)
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		key := parts[0]
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		if key == "worktree" {
			flush()
			current = &Worktree{Path: value, Name: filepath.Base(value), Branch: "detached"}
			continue
		}
		if current == nil {
			continue
		}
		switch key {
		case "HEAD":
			if len(value) > 8 {
				current.Head = value[:8]
			} else {
				current.Head = value
			}
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			current.Detached = true
		case "bare":
			current.Bare = true
		}
	}
	flush()
	return worktrees
}

func gitStateForPath(path string) GitState {
	out, err := git(path, "status", "--short", "--branch", "--untracked-files=no")
	if err != nil {
		return GitState{Clean: true}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	dirtyFiles := 0
	if len(lines) > 0 {
		dirtyFiles = len(lines) - 1
	}
	header := ""
	if len(lines) > 0 {
		header = lines[0]
	}
	ahead := extractCount(header, "ahead")
	behind := extractCount(header, "behind")
	return GitState{
		DirtyFiles: dirtyFiles,
		Ahead:      ahead,
		Behind:     behind,
		Clean:      dirtyFiles == 0,
	}
}

func activityForPath(path string, activity map[string]Activity) Activity {
	if len(activity) == 0 {
		return Activity{State: "idle"}
	}
	cleanPath := filepath.Clean(path)
	best := Activity{State: "idle"}
	bestLen := -1
	for eventPath, item := range activity {
		cleanEventPath := filepath.Clean(eventPath)
		if cleanPath == cleanEventPath || strings.HasPrefix(cleanEventPath, cleanPath+string(os.PathSeparator)) {
			if len(cleanEventPath) > bestLen {
				best = item
				bestLen = len(cleanEventPath)
			}
		}
	}
	if best.State == "" {
		best.State = "idle"
	}
	return best
}

func extractCount(header, key string) int {
	start := strings.Index(header, key+" ")
	if start < 0 {
		return 0
	}
	start += len(key) + 1
	end := start
	for end < len(header) && header[end] >= '0' && header[end] <= '9' {
		end++
	}
	var n int
	fmt.Sscanf(header[start:end], "%d", &n)
	return n
}
