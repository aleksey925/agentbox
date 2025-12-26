package config

import (
	"encoding/json"
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
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Agents == nil {
		state.Agents = make(map[string]*AgentState)
	}

	return &state, nil
}

func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
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
