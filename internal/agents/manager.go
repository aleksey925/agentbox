package agents

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/aleksey925/agentbox/internal/config"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

const maxVersionsToKeep = 5

type Manager struct {
	paths  *config.Paths
	state  *config.State
	agents map[string]Agent
}

func NewManager(paths *config.Paths, state *config.State) (*Manager, error) {
	claude, err := NewClaudeAgent()
	if err != nil {
		return nil, err
	}
	copilot, err := NewCopilotAgent()
	if err != nil {
		return nil, err
	}
	codex, err := NewCodexAgent()
	if err != nil {
		return nil, err
	}

	return &Manager{
		paths: paths,
		state: state,
		agents: map[string]Agent{
			"claude":  claude,
			"copilot": copilot,
			"codex":   codex,
			"gemini":  NewGeminiAgent(),
		},
	}, nil
}

func (m *Manager) GetAgent(name string) (Agent, bool) {
	agent, ok := m.agents[name]
	return agent, ok
}

func (m *Manager) AllAgents() []Agent {
	return []Agent{
		m.agents["claude"],
		m.agents["copilot"],
		m.agents["codex"],
		m.agents["gemini"],
	}
}

type AgentStatus struct {
	Name      string
	Installed string
	Latest    string
	UpToDate  bool
	Error     error
}

func (m *Manager) GetStatus() []AgentStatus {
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([]AgentStatus, len(m.agents))
	names := AllAgentNames()

	for i, name := range names {
		wg.Add(1)
		go func(idx int, agentName string) {
			defer wg.Done()

			agent := m.agents[agentName]
			status := AgentStatus{Name: agentName}

			installed := m.state.GetAgentVersion(agentName)
			status.Installed = installed

			latest, err := agent.FetchLatestVersion(ctx)
			if err != nil {
				status.Error = err
			} else {
				status.Latest = latest
				status.UpToDate = installed == latest
			}

			results[idx] = status
		}(i, name)
	}

	wg.Wait()
	return results
}

func (m *Manager) Install(name string, onProgress func(agent string, downloaded, total int64)) error {
	ctx := context.Background()
	agent, ok := m.agents[name]
	if !ok {
		return fmt.Errorf("unknown agent: %s", name)
	}

	version, err := agent.FetchLatestVersion(ctx)
	if err != nil {
		return fmt.Errorf("fetch latest version: %w", err)
	}

	destDir := m.paths.AgentVersionDir(name, version)

	if _, err := os.Stat(destDir); err == nil {
		if err := m.switchVersion(name, version); err != nil {
			return err
		}
		return nil
	}

	progress := func(downloaded, total int64) {
		if onProgress != nil {
			onProgress(name, downloaded, total)
		}
	}

	if err := agent.Download(ctx, version, destDir, progress); err != nil {
		os.RemoveAll(destDir)
		return fmt.Errorf("download: %w", err)
	}

	if err := m.switchVersion(name, version); err != nil {
		return err
	}

	m.state.SetAgent(name, version, agent.Variant())

	return nil
}

func (m *Manager) Update(names []string) ([]DownloadResult, error) {
	ctx := context.Background()
	if len(names) == 0 {
		names = AllAgentNames()
	}

	// create multi-progress container
	p := mpb.New(mpb.WithWidth(60))

	results := make([]DownloadResult, len(names))
	var wg sync.WaitGroup

	for i, name := range names {
		wg.Add(1)
		go func(idx int, agentName string) {
			defer wg.Done()

			agent, ok := m.agents[agentName]
			if !ok {
				results[idx] = DownloadResult{
					Agent: agentName,
					Error: fmt.Errorf("unknown agent: %s", agentName),
				}
				return
			}

			version, err := agent.FetchLatestVersion(ctx)
			if err != nil {
				results[idx] = DownloadResult{
					Agent: agentName,
					Error: err,
				}
				return
			}

			destDir := m.paths.AgentVersionDir(agentName, version)

			// already installed
			if _, err := os.Stat(destDir); err == nil {
				results[idx] = DownloadResult{
					Agent:   agentName,
					Version: version,
					Variant: agent.Variant(),
				}
				return
			}

			// create progress bar for this agent
			bar := p.AddBar(0,
				mpb.PrependDecorators(
					decor.Name(fmt.Sprintf("  %-8s", agentName)),
					decor.CountersKibiByte(" %6.1f / %6.1f", decor.WCSyncSpace),
				),
				mpb.AppendDecorators(
					decor.NewPercentage("%3d%%", decor.WC{W: 5}),
					decor.AverageETA(decor.ET_STYLE_GO, decor.WC{W: 8}),
					decor.AverageSpeed(decor.SizeB1024(0), "%6.1f", decor.WC{W: 12}),
				),
			)

			var lastDownloaded int64
			progress := func(downloaded, total int64) {
				if total > 0 {
					bar.SetTotal(total, false)
				}
				increment := downloaded - lastDownloaded
				if increment > 0 {
					bar.IncrInt64(increment)
					lastDownloaded = downloaded
				}
			}

			if err := agent.Download(ctx, version, destDir, progress); err != nil {
				bar.Abort(true)
				os.RemoveAll(destDir)
				results[idx] = DownloadResult{
					Agent: agentName,
					Error: err,
				}
				return
			}

			bar.SetTotal(bar.Current(), true)

			results[idx] = DownloadResult{
				Agent:   agentName,
				Version: version,
				Variant: agent.Variant(),
			}
		}(i, name)
	}

	wg.Wait()
	p.Wait()

	m.applyResults(results)

	return results, nil
}

func (m *Manager) applyResults(results []DownloadResult) {
	for i := range results {
		if results[i].Error == nil && results[i].Version != "" {
			if err := m.switchVersion(results[i].Agent, results[i].Version); err != nil {
				results[i].Error = fmt.Errorf("switch version: %w", err)
				continue
			}
			m.state.SetAgent(results[i].Agent, results[i].Version, results[i].Variant)
		}
	}
}

func (m *Manager) SwitchVersion(name, version string) error {
	if _, ok := m.agents[name]; !ok {
		return fmt.Errorf("unknown agent: %s", name)
	}

	versionDir := m.paths.AgentVersionDir(name, version)
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		return fmt.Errorf("version %s not installed for %s", version, name)
	}

	if err := m.switchVersion(name, version); err != nil {
		return err
	}

	agent := m.agents[name]
	m.state.SetAgent(name, version, agent.Variant())

	return nil
}

func (m *Manager) switchVersion(name, version string) error {
	currentFile := m.paths.AgentCurrentFile(name)

	if err := os.Remove(currentFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove current: %w", err)
	}

	// write version to file instead of symlink to avoid Docker VM cache issues
	if err := os.WriteFile(currentFile, []byte(version+"\n"), 0o644); err != nil {
		return fmt.Errorf("write current version: %w", err)
	}
	return nil
}

func (m *Manager) Cleanup(name string) (int, error) {
	agentDir := m.paths.AgentDir(name)

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read agent dir: %w", err)
	}

	var versions []string
	for _, entry := range entries {
		if entry.Name() == "current" {
			continue
		}
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	if len(versions) <= maxVersionsToKeep {
		return 0, nil
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})

	toRemove := versions[maxVersionsToKeep:]
	removed := 0

	for _, v := range toRemove {
		versionDir := m.paths.AgentVersionDir(name, v)
		if err := os.RemoveAll(versionDir); err != nil {
			continue
		}
		removed++
	}

	return removed, nil
}

func (m *Manager) ListVersions(name string) ([]string, string, error) {
	agentDir := m.paths.AgentDir(name)

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("read agent dir: %w", err)
	}

	var versions []string
	var current string

	currentFile := m.paths.AgentCurrentFile(name)
	if data, err := os.ReadFile(currentFile); err == nil {
		current = strings.TrimSpace(string(data))
	}

	for _, entry := range entries {
		if entry.Name() == "current" {
			continue
		}
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})

	return versions, current, nil
}

func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := max(len(partsA), len(partsB))

	for i := range maxLen {
		var numA, numB int
		if i < len(partsA) {
			_, _ = fmt.Sscanf(partsA[i], "%d", &numA)
		}
		if i < len(partsB) {
			_, _ = fmt.Sscanf(partsB[i], "%d", &numB)
		}

		if numA > numB {
			return 1
		}
		if numA < numB {
			return -1
		}
	}

	return 0
}
