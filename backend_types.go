package main

type State struct {
	Config      Config      `json:"config"`
	Projects    []ProjectVM `json:"projects"`
	Scanning    bool        `json:"scanning"`
	LastUpdated string      `json:"lastUpdated"`
	Hook        HookStatus  `json:"hook"`
}

type ProjectVM struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Root      string     `json:"root"`
	Worktrees []Worktree `json:"worktrees"`
	Error     string     `json:"error,omitempty"`
}

type Worktree struct {
	Path        string   `json:"path"`
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Branch      string   `json:"branch"`
	Head        string   `json:"head"`
	Detached    bool     `json:"detached"`
	Bare        bool     `json:"bare"`
	Status      GitState `json:"status"`
	Activity    Activity `json:"activity"`
}

type GitState struct {
	DirtyFiles int  `json:"dirtyFiles"`
	Ahead      int  `json:"ahead"`
	Behind     int  `json:"behind"`
	Clean      bool `json:"clean"`
}

type Activity struct {
	State     string `json:"state"`
	Source    string `json:"source"`
	ChangedAt string `json:"changedAt"`
	EventPath string `json:"eventPath,omitempty"`
	EventName string `json:"eventName,omitempty"`
}

type HookStatus struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Error   string `json:"error,omitempty"`
}
