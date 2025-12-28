package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type AgentState struct {
	Version     string    `json:"version"`
	Variant     string    `json:"variant"`
	InstalledAt time.Time `json:"installed_at"`
}

type State struct {
	Arch   string                 `json:"arch"`
	Agents map[string]*AgentState `json:"agents"`
}

func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				Agents: make(map[string]*AgentState),
			}, nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	if state.Agents == nil {
		state.Agents = make(map[string]*AgentState)
	}

	return &state, nil
}

func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

func (s *State) SetAgent(name, version, variant string) {
	s.Agents[name] = &AgentState{
		Version:     version,
		Variant:     variant,
		InstalledAt: time.Now().UTC(),
	}
}

func (s *State) GetAgentVersion(name string) string {
	if agent, ok := s.Agents[name]; ok {
		return agent.Version
	}
	return ""
}
