package vault

import (
	"sync"
	"testing"
)

func TestListVaultsDoesNotShareEntryStorage(t *testing.T) {
	s := NewService()
	v := s.CreateVault("Vault", "")
	v.Entries = []Entry{{Key: "SECRET", ValueDev: "original"}}
	s.UpdateVault(v)

	// A caller mutating what it got back must not reach into the service.
	listed := s.ListVaults()
	if len(listed) != 1 || len(listed[0].Entries) != 1 {
		t.Fatalf("unexpected listing: %+v", listed)
	}
	listed[0].Entries[0].ValueDev = "tampered"

	again := s.ListVaults()
	if got := again[0].Entries[0].ValueDev; got != "original" {
		t.Errorf("stored value = %q, want %q; the entry slice is still shared with callers", got, "original")
	}
}

func TestGetVaultDoesNotShareEntryStorage(t *testing.T) {
	s := NewService()
	v := s.CreateVault("Vault", "")
	v.Entries = []Entry{{Key: "SECRET", ValueDev: "original"}}
	s.UpdateVault(v)

	got, ok := s.GetVault(v.ID)
	if !ok {
		t.Fatal("vault not found")
	}
	got.Entries[0].ValueDev = "tampered"

	fresh, _ := s.GetVault(v.ID)
	if fresh.Entries[0].ValueDev != "original" {
		t.Error("GetVault handed out a copy that still shares its entry slice")
	}
}

func TestSetAllDoesNotShareEntryStorage(t *testing.T) {
	s := NewService()
	input := []*Vault{{ID: "id-1", Name: "V", Entries: []Entry{{Key: "K", ValueDev: "original"}}}}
	s.SetAll(input)

	input[0].Entries[0].ValueDev = "tampered"

	got, _ := s.GetVault("id-1")
	if got.Entries[0].ValueDev != "original" {
		t.Error("SetAll stored the caller's entry slice instead of a copy")
	}
}

func TestListVaultsIsSorted(t *testing.T) {
	s := NewService()
	for _, name := range []string{"Zulu", "Alpha", "Mike"} {
		s.CreateVault(name, "")
	}

	// Repeat: Go randomises map iteration, so an unsorted implementation
	// would only fail intermittently.
	for i := 0; i < 20; i++ {
		got := s.ListVaults()
		if len(got) != 3 {
			t.Fatalf("got %d vaults, want 3", len(got))
		}
		if got[0].Name != "Alpha" || got[1].Name != "Mike" || got[2].Name != "Zulu" {
			t.Fatalf("unsorted listing: %s, %s, %s", got[0].Name, got[1].Name, got[2].Name)
		}
	}
}

func TestRestoreVault(t *testing.T) {
	s := NewService()
	v := s.CreateVault("Vault", "desc")
	v.Entries = []Entry{{Key: "K", ValueDev: "v"}}
	s.UpdateVault(v)

	snapshot, _ := s.GetVault(v.ID)

	if !s.DeleteVault(v.ID) {
		t.Fatal("delete failed")
	}
	if _, ok := s.GetVault(v.ID); ok {
		t.Fatal("vault still present after delete")
	}

	if !s.RestoreVault(snapshot) {
		t.Fatal("restore failed")
	}

	got, ok := s.GetVault(v.ID)
	if !ok {
		t.Fatal("vault missing after restore")
	}
	if got.Name != "Vault" || len(got.Entries) != 1 || got.Entries[0].ValueDev != "v" {
		t.Errorf("restored vault does not match the snapshot: %+v", got)
	}

	if s.RestoreVault(nil) {
		t.Error("RestoreVault(nil) should report failure")
	}
	if s.RestoreVault(&Vault{}) {
		t.Error("RestoreVault should reject a vault without an ID")
	}
}

func TestUpdateVaultRejectsUnknownID(t *testing.T) {
	s := NewService()
	if s.UpdateVault(&Vault{ID: "does-not-exist"}) {
		t.Error("UpdateVault should not create a vault that was never added")
	}
	if s.UpdateVault(nil) {
		t.Error("UpdateVault(nil) should report failure")
	}
}

func TestConcurrentAccessIsSafe(t *testing.T) {
	s := NewService()
	v := s.CreateVault("Vault", "")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.ListVaults()
			s.GetVault(v.ID)
			s.UpdateVault(&Vault{ID: v.ID, Name: "Vault", Entries: []Entry{{Key: "K"}}})
		}()
	}
	wg.Wait()
}
