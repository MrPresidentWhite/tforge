package vault

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

type EntryType string

const (
	EntryTypeEnv    EntryType = "env"
	EntryTypeSecret EntryType = "secret"
	EntryTypeNote   EntryType = "note"
)

type Entry struct {
	Key         string    `json:"key"`
	ValueDev    string    `json:"valueDev,omitempty"`
	ValueStage  string    `json:"valueStage,omitempty"`
	ValueProd   string    `json:"valueProd,omitempty"`
	Type        EntryType `json:"type"`
	GroupPrefix string    `json:"groupPrefix,omitempty"`
}

type Vault struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon,omitempty"`
	Description string    `json:"description,omitempty"`
	Entries     []Entry   `json:"entries"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Service struct {
	mu     sync.RWMutex
	vaults map[string]*Vault
}

// clone returns a deep copy of v. Copying the struct alone is not enough:
// Entries is a slice, so a plain struct copy would still share the backing
// array and let callers mutate stored entries in place.
func clone(v *Vault) *Vault {
	if v == nil {
		return nil
	}
	copied := *v
	copied.Entries = append([]Entry(nil), v.Entries...)
	return &copied
}

func NewService() *Service {
	return &Service{
		vaults: make(map[string]*Vault),
	}
}

func (s *Service) ListVaults() []*Vault {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Vault, 0, len(s.vaults))
	for _, v := range s.vaults {
		result = append(result, clone(v))
	}
	// Stable ordering keeps CLI output and the GUI list deterministic; map
	// iteration order in Go is deliberately randomised.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// SetAll ersetzt den kompletten Vault-State (z.B. nach dem Laden von Disk).
func (s *Service) SetAll(vaults []*Vault) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.vaults = make(map[string]*Vault, len(vaults))
	for _, v := range vaults {
		if v == nil || v.ID == "" {
			continue
		}
		copied := clone(v)
		// Normalise on the way in as well, so what is displayed already
		// follows the rule. This only touches the in-memory copy; the file on
		// disk is rewritten when the user next saves something, not just for
		// having opened the app.
		NormalizeGroups(copied.Entries)
		s.vaults[v.ID] = copied
	}
}

// RestoreVault re-inserts a previously removed or modified vault, keeping its
// original ID. It is used to roll back an in-memory change when persisting it
// to disk failed.
func (s *Service) RestoreVault(v *Vault) bool {
	if v == nil || v.ID == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.vaults[v.ID] = clone(v)
	return true
}

func (s *Service) CreateVault(name, description string) *Vault {
	s.mu.Lock()
	defer s.mu.Unlock()

	v := &Vault{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		Entries:     []Entry{},
		UpdatedAt:   time.Now().UTC(),
	}
	s.vaults[v.ID] = v
	return v
}

func (s *Service) GetVault(id string) (*Vault, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.vaults[id]
	if !ok {
		return nil, false
	}
	return clone(v), true
}

func (s *Service) UpdateVault(updated *Vault) bool {
	if updated == nil || updated.ID == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.vaults[updated.ID]
	if !ok {
		return false
	}

	existing.Name = updated.Name
	existing.Icon = updated.Icon
	existing.Description = updated.Description
	existing.Entries = append([]Entry(nil), updated.Entries...)
	// Every edit funnels through here, which makes this the one place where
	// grouping can be kept honest: a group that lost members below the
	// minimum dissolves, and keys that now share a prefix come together.
	NormalizeGroups(existing.Entries)
	existing.UpdatedAt = time.Now().UTC()

	return true
}

func (s *Service) DeleteVault(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.vaults[id]; !ok {
		return false
	}
	delete(s.vaults, id)
	return true
}
