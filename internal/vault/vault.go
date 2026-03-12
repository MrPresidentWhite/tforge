package vault

import (
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
		// return shallow copies to avoid external mutation
		copyVault := *v
		result = append(result, &copyVault)
	}
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
		copyVault := *v
		s.vaults[v.ID] = &copyVault
	}
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
	copyVault := *v
	return &copyVault, true
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

