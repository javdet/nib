package static

import (
	"sync/atomic"
	"time"

	"github.com/javdet/nib/internal/config"
	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// Deterministic namespace for generating stable UUIDs from config names.
var configNamespace = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

const anyName = "any"

func deterministicID(namespace uuid.UUID, name string) uuid.UUID {
	return uuid.NewSHA1(namespace, []byte(name))
}

// Store holds all domain objects built from the YAML config.
// Each Store instance is immutable once created; mutations produce a new Store.
type Store struct {
	Projects     []domain.Project
	Environments []domain.Environment
	Clouds       []domain.Cloud
	Locations    []domain.Location
}

// StoreHolder wraps an atomic pointer so readers get lock-free access to the
// current Store while writers can swap in a freshly rebuilt one.
type StoreHolder struct {
	ptr atomic.Pointer[Store]
}

func NewStoreHolder(s *Store) *StoreHolder {
	h := &StoreHolder{}
	h.ptr.Store(s)
	return h
}

func (h *StoreHolder) Load() *Store  { return h.ptr.Load() }
func (h *StoreHolder) Swap(s *Store) { h.ptr.Store(s) }

// NewStore builds all domain objects from the parsed config, generating
// deterministic UUIDs so the same config always produces the same IDs.
// An "any" entry is automatically prepended at every level:
// project, environment (per project), cloud (per project), and location (per cloud).
func NewStore(projects []config.ProjectConfig) *Store {
	now := time.Now().UTC()
	s := &Store{}

	// Global "any" project with its own "any" environment, cloud, and location.
	anyProjectID := deterministicID(configNamespace, anyName)
	s.Projects = append(s.Projects, domain.Project{
		ID:          anyProjectID,
		Name:        anyName,
		Description: "Any project",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	s.addAnyEnvironment(anyProjectID, now)
	anyCloudID := s.addAnyCloud(anyProjectID, now)
	s.addAnyLocation(anyProjectID, anyCloudID, now)

	for _, pc := range projects {
		projectID := deterministicID(configNamespace, pc.Name)

		s.Projects = append(s.Projects, domain.Project{
			ID:          projectID,
			Name:        pc.Name,
			Description: pc.Description,
			CreatedAt:   now,
			UpdatedAt:   now,
		})

		// "any" environment for this project, then the configured ones.
		s.addAnyEnvironment(projectID, now)
		for _, ec := range pc.Environments {
			envID := deterministicID(projectID, ec.Name)
			s.Environments = append(s.Environments, domain.Environment{
				ID:          envID,
				ProjectID:   projectID,
				Name:        ec.Name,
				Description: ec.Description,
				CreatedAt:   now,
				UpdatedAt:   now,
			})
		}

		// "any" cloud (with "any" location) for this project, then configured clouds.
		anyCloudID := s.addAnyCloud(projectID, now)
		s.addAnyLocation(projectID, anyCloudID, now)

		for _, cc := range pc.Clouds {
			cloudID := deterministicID(projectID, cc.Name)
			s.Clouds = append(s.Clouds, domain.Cloud{
				ID:          cloudID,
				ProjectID:   projectID,
				Name:        cc.Name,
				Description: cc.Description,
				CreatedAt:   now,
				UpdatedAt:   now,
			})

			// "any" location for this cloud, then configured locations.
			s.addAnyLocation(projectID, cloudID, now)
			for _, lc := range cc.Locations {
				locID := deterministicID(cloudID, lc.Name)
				s.Locations = append(s.Locations, domain.Location{
					ID:          locID,
					ProjectID:   projectID,
					CloudID:     cloudID,
					Name:        lc.Name,
					Description: lc.Description,
					CreatedAt:   now,
					UpdatedAt:   now,
				})
			}
		}
	}

	return s
}

func (s *Store) addAnyEnvironment(projectID uuid.UUID, now time.Time) {
	envID := deterministicID(projectID, anyName)
	s.Environments = append(s.Environments, domain.Environment{
		ID:          envID,
		ProjectID:   projectID,
		Name:        anyName,
		Description: "Any environment",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func (s *Store) addAnyCloud(projectID uuid.UUID, now time.Time) uuid.UUID {
	cloudID := deterministicID(projectID, anyName)
	s.Clouds = append(s.Clouds, domain.Cloud{
		ID:          cloudID,
		ProjectID:   projectID,
		Name:        anyName,
		Description: "Any cloud",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	return cloudID
}

func (s *Store) addAnyLocation(projectID, cloudID uuid.UUID, now time.Time) {
	locID := deterministicID(cloudID, anyName)
	s.Locations = append(s.Locations, domain.Location{
		ID:          locID,
		ProjectID:   projectID,
		CloudID:     cloudID,
		Name:        anyName,
		Description: "Any location",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}
