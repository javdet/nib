package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	ErrDuplicate = errors.New("already exists")
	ErrNotFound  = errors.New("not found in config")
)

// Manager provides thread-safe CRUD operations on the projects config tree
// and persists every mutation atomically to the YAML file on disk.
// After each write it invokes the onUpdate callback so callers can rebuild
// derived state (e.g. the static Store).
type Manager struct {
	mu         sync.Mutex
	configPath string
	projects   []ProjectConfig
	onUpdate   func([]ProjectConfig)
}

// NewManager creates a Manager seeded with the projects loaded at startup.
// onUpdate is called (under the lock) after every successful mutation + save.
func NewManager(configPath string, projects []ProjectConfig, onUpdate func([]ProjectConfig)) *Manager {
	cp := make([]ProjectConfig, len(projects))
	copy(cp, projects)
	return &Manager{
		configPath: configPath,
		projects:   cp,
		onUpdate:   onUpdate,
	}
}

// Projects returns a deep copy of the current project configs.
func (m *Manager) Projects() []ProjectConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return copyProjects(m.projects)
}

// --- Project CRUD ---

func (m *Manager) AddProject(p ProjectConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.projects {
		if existing.Name == p.Name {
			return fmt.Errorf("project %q: %w", p.Name, ErrDuplicate)
		}
	}
	m.projects = append(m.projects, p)
	return m.saveAndNotify()
}

func (m *Manager) UpdateProject(name string, p ProjectConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.findProject(name)
	if idx < 0 {
		return fmt.Errorf("project %q: %w", name, ErrNotFound)
	}
	if p.Name != name {
		for i, existing := range m.projects {
			if i != idx && existing.Name == p.Name {
				return fmt.Errorf("project %q: %w", p.Name, ErrDuplicate)
			}
		}
	}
	m.projects[idx].Name = p.Name
	m.projects[idx].Description = p.Description
	return m.saveAndNotify()
}

func (m *Manager) RemoveProject(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.findProject(name)
	if idx < 0 {
		return fmt.Errorf("project %q: %w", name, ErrNotFound)
	}
	m.projects = append(m.projects[:idx], m.projects[idx+1:]...)
	return m.saveAndNotify()
}

// --- Environment CRUD ---

func (m *Manager) AddEnvironment(projectName string, e EnvironmentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.findProject(projectName)
	if idx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	for _, existing := range m.projects[idx].Environments {
		if existing.Name == e.Name {
			return fmt.Errorf("environment %q in project %q: %w", e.Name, projectName, ErrDuplicate)
		}
	}
	m.projects[idx].Environments = append(m.projects[idx].Environments, e)
	return m.saveAndNotify()
}

func (m *Manager) UpdateEnvironment(projectName, envName string, e EnvironmentConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	eIdx := findByName(envConfigNames(m.projects[pIdx].Environments), envName)
	if eIdx < 0 {
		return fmt.Errorf("environment %q in project %q: %w", envName, projectName, ErrNotFound)
	}
	if e.Name != envName {
		for i, existing := range m.projects[pIdx].Environments {
			if i != eIdx && existing.Name == e.Name {
				return fmt.Errorf("environment %q in project %q: %w", e.Name, projectName, ErrDuplicate)
			}
		}
	}
	m.projects[pIdx].Environments[eIdx] = e
	return m.saveAndNotify()
}

func (m *Manager) RemoveEnvironment(projectName, envName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	eIdx := findByName(envConfigNames(m.projects[pIdx].Environments), envName)
	if eIdx < 0 {
		return fmt.Errorf("environment %q in project %q: %w", envName, projectName, ErrNotFound)
	}
	envs := m.projects[pIdx].Environments
	m.projects[pIdx].Environments = append(envs[:eIdx], envs[eIdx+1:]...)
	return m.saveAndNotify()
}

// --- Cloud CRUD ---

func (m *Manager) AddCloud(projectName string, c CloudConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	for _, existing := range m.projects[pIdx].Clouds {
		if existing.Name == c.Name {
			return fmt.Errorf("cloud %q in project %q: %w", c.Name, projectName, ErrDuplicate)
		}
	}
	m.projects[pIdx].Clouds = append(m.projects[pIdx].Clouds, c)
	return m.saveAndNotify()
}

func (m *Manager) UpdateCloud(projectName, cloudName string, c CloudConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	cIdx := findByName(cloudConfigNames(m.projects[pIdx].Clouds), cloudName)
	if cIdx < 0 {
		return fmt.Errorf("cloud %q in project %q: %w", cloudName, projectName, ErrNotFound)
	}
	if c.Name != cloudName {
		for i, existing := range m.projects[pIdx].Clouds {
			if i != cIdx && existing.Name == c.Name {
				return fmt.Errorf("cloud %q in project %q: %w", c.Name, projectName, ErrDuplicate)
			}
		}
	}
	m.projects[pIdx].Clouds[cIdx].Name = c.Name
	m.projects[pIdx].Clouds[cIdx].Description = c.Description
	return m.saveAndNotify()
}

func (m *Manager) RemoveCloud(projectName, cloudName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	cIdx := findByName(cloudConfigNames(m.projects[pIdx].Clouds), cloudName)
	if cIdx < 0 {
		return fmt.Errorf("cloud %q in project %q: %w", cloudName, projectName, ErrNotFound)
	}
	clouds := m.projects[pIdx].Clouds
	m.projects[pIdx].Clouds = append(clouds[:cIdx], clouds[cIdx+1:]...)
	return m.saveAndNotify()
}

// --- Location CRUD ---

func (m *Manager) AddLocation(projectName, cloudName string, l LocationConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	cIdx := findByName(cloudConfigNames(m.projects[pIdx].Clouds), cloudName)
	if cIdx < 0 {
		return fmt.Errorf("cloud %q in project %q: %w", cloudName, projectName, ErrNotFound)
	}
	for _, existing := range m.projects[pIdx].Clouds[cIdx].Locations {
		if existing.Name == l.Name {
			return fmt.Errorf("location %q in cloud %q: %w", l.Name, cloudName, ErrDuplicate)
		}
	}
	m.projects[pIdx].Clouds[cIdx].Locations = append(m.projects[pIdx].Clouds[cIdx].Locations, l)
	return m.saveAndNotify()
}

func (m *Manager) UpdateLocation(projectName, cloudName, locName string, l LocationConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	cIdx := findByName(cloudConfigNames(m.projects[pIdx].Clouds), cloudName)
	if cIdx < 0 {
		return fmt.Errorf("cloud %q in project %q: %w", cloudName, projectName, ErrNotFound)
	}
	lIdx := findByName(locConfigNames(m.projects[pIdx].Clouds[cIdx].Locations), locName)
	if lIdx < 0 {
		return fmt.Errorf("location %q in cloud %q: %w", locName, cloudName, ErrNotFound)
	}
	if l.Name != locName {
		for i, existing := range m.projects[pIdx].Clouds[cIdx].Locations {
			if i != lIdx && existing.Name == l.Name {
				return fmt.Errorf("location %q in cloud %q: %w", l.Name, cloudName, ErrDuplicate)
			}
		}
	}
	m.projects[pIdx].Clouds[cIdx].Locations[lIdx] = l
	return m.saveAndNotify()
}

func (m *Manager) RemoveLocation(projectName, cloudName, locName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pIdx := m.findProject(projectName)
	if pIdx < 0 {
		return fmt.Errorf("project %q: %w", projectName, ErrNotFound)
	}
	cIdx := findByName(cloudConfigNames(m.projects[pIdx].Clouds), cloudName)
	if cIdx < 0 {
		return fmt.Errorf("cloud %q in project %q: %w", cloudName, projectName, ErrNotFound)
	}
	lIdx := findByName(locConfigNames(m.projects[pIdx].Clouds[cIdx].Locations), locName)
	if lIdx < 0 {
		return fmt.Errorf("location %q in cloud %q: %w", locName, cloudName, ErrNotFound)
	}
	locs := m.projects[pIdx].Clouds[cIdx].Locations
	m.projects[pIdx].Clouds[cIdx].Locations = append(locs[:lIdx], locs[lIdx+1:]...)
	return m.saveAndNotify()
}

// --- internal helpers ---

func (m *Manager) findProject(name string) int {
	for i, p := range m.projects {
		if p.Name == name {
			return i
		}
	}
	return -1
}

func (m *Manager) saveAndNotify() error {
	if err := m.save(); err != nil {
		return err
	}
	if m.onUpdate != nil {
		m.onUpdate(copyProjects(m.projects))
	}
	return nil
}

// save writes the config atomically: temp file in same dir, then rename.
func (m *Manager) save() error {
	pf := fileConfig{Projects: m.projects}
	data, err := yaml.Marshal(&pf)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(m.configPath)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config file: %w", err)
	}
	if err := os.Rename(tmpName, m.configPath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp config file: %w", err)
	}
	return nil
}

// name extraction helpers for the generic findByName.
func findByName(names []string, target string) int {
	for i, n := range names {
		if n == target {
			return i
		}
	}
	return -1
}

func envConfigNames(envs []EnvironmentConfig) []string {
	names := make([]string, len(envs))
	for i, e := range envs {
		names[i] = e.Name
	}
	return names
}

func cloudConfigNames(clouds []CloudConfig) []string {
	names := make([]string, len(clouds))
	for i, c := range clouds {
		names[i] = c.Name
	}
	return names
}

func locConfigNames(locs []LocationConfig) []string {
	names := make([]string, len(locs))
	for i, l := range locs {
		names[i] = l.Name
	}
	return names
}

func copyProjects(src []ProjectConfig) []ProjectConfig {
	dst := make([]ProjectConfig, len(src))
	for i, p := range src {
		dst[i] = ProjectConfig{
			Name:        p.Name,
			Description: p.Description,
		}
		if len(p.Environments) > 0 {
			dst[i].Environments = make([]EnvironmentConfig, len(p.Environments))
			copy(dst[i].Environments, p.Environments)
		}
		if len(p.Clouds) > 0 {
			dst[i].Clouds = make([]CloudConfig, len(p.Clouds))
			for j, c := range p.Clouds {
				dst[i].Clouds[j] = CloudConfig{
					Name:        c.Name,
					Description: c.Description,
				}
				if len(c.Locations) > 0 {
					dst[i].Clouds[j].Locations = make([]LocationConfig, len(c.Locations))
					copy(dst[i].Clouds[j].Locations, c.Locations)
				}
			}
		}
	}
	return dst
}
