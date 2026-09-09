package main

import (
	"reflect"
	"testing"

	"tforge/internal/vault"
)

// The example from the feature request, verbatim.
const exampleEnv = `POSTGRES_HOST=localhost
POSTGRES_USER=cinevault
POSTGRES_PASSWORD=
POSTGRES_DB=cinevault
POSTGRES_PORT=5432
`

func newImportApp() *App {
	return &App{vaults: vault.NewService(), protector: &stubProtector{}}
}

func TestAnalyseEnvImportGroupsSharedPrefix(t *testing.T) {
	app := newImportApp()

	got, err := app.AnalyseEnvImport(exampleEnv, "", "", "")
	if err != nil {
		t.Fatalf("AnalyseEnvImport: %v", err)
	}

	if len(got.Keys) != 5 {
		t.Fatalf("structure holds %d keys, want 5", len(got.Keys))
	}
	if len(got.Groups) != 1 {
		t.Fatalf("got %d groups, want 1: %+v", len(got.Groups), got.Groups)
	}
	if got.Groups[0].Prefix != "POSTGRES_" {
		t.Errorf("prefix = %q, want POSTGRES_", got.Groups[0].Prefix)
	}
	if len(got.Ungrouped) != 0 {
		t.Errorf("ungrouped = %v, want none", got.Ungrouped)
	}

	// No environment files were supplied, so nothing should be flagged.
	for name, report := range got.Envs {
		if report.Provided {
			t.Errorf("%s reported as provided although no file was given", name)
		}
	}
}

func TestAnalyseEnvImportLeavesLoneKeysUngrouped(t *testing.T) {
	example := "POSTGRES_HOST=\nPOSTGRES_PORT=\nREDIS_URL=\nDEBUG=\n"

	got, err := newImportApp().AnalyseEnvImport(example, "", "", "")
	if err != nil {
		t.Fatalf("AnalyseEnvImport: %v", err)
	}

	if len(got.Groups) != 1 || got.Groups[0].Prefix != "POSTGRES_" {
		t.Fatalf("groups = %+v, want only POSTGRES_", got.Groups)
	}
	if len(got.Ungrouped) != 2 {
		t.Errorf("ungrouped = %v, want REDIS_URL and DEBUG", got.Ungrouped)
	}
}

func TestAnalyseEnvImportValidatesEachEnvironment(t *testing.T) {
	dev := `POSTGRES_HOST=localhost
POSTGRES_USER=cinevault
POSTGRES_DB=cinevault
POSTGRES_PORT=5432
STRAY_KEY=surprise
`

	got, err := newImportApp().AnalyseEnvImport(exampleEnv, dev, "", "")
	if err != nil {
		t.Fatalf("AnalyseEnvImport: %v", err)
	}

	devReport := got.Envs["dev"]
	if !devReport.Provided {
		t.Fatal("dev should be marked as provided")
	}
	if !reflect.DeepEqual(devReport.Missing, []string{"POSTGRES_PASSWORD"}) {
		t.Errorf("missing = %v, want POSTGRES_PASSWORD", devReport.Missing)
	}
	if !reflect.DeepEqual(devReport.Unknown, []string{"STRAY_KEY"}) {
		t.Errorf("unknown = %v, want STRAY_KEY", devReport.Unknown)
	}
	if devReport.Values["POSTGRES_HOST"] != "localhost" {
		t.Errorf("values not carried over: %+v", devReport.Values)
	}
	if _, ok := devReport.Values["POSTGRES_PASSWORD"]; ok {
		t.Error("an empty value was carried over as if it had been supplied")
	}
}

func TestAnalyseEnvImportIgnoresLineOrder(t *testing.T) {
	forward := "A=1\nB=2\nC=3\n"
	shuffled := "C=3\nA=1\nB=2\n"
	structure := "A=\nB=\nC=\n"

	app := newImportApp()

	a, err := app.AnalyseEnvImport(structure, forward, "", "")
	if err != nil {
		t.Fatalf("AnalyseEnvImport: %v", err)
	}
	b, err := app.AnalyseEnvImport(structure, shuffled, "", "")
	if err != nil {
		t.Fatalf("AnalyseEnvImport: %v", err)
	}

	if len(a.Envs["dev"].Missing) != 0 || len(a.Envs["dev"].Unknown) != 0 {
		t.Errorf("in-order file reported issues: %+v", a.Envs["dev"])
	}
	if !reflect.DeepEqual(a.Envs["dev"], b.Envs["dev"]) {
		t.Errorf("reordering the file changed the result:\n%+v\n%+v", a.Envs["dev"], b.Envs["dev"])
	}
}

func TestAnalyseEnvImportRejectsAnEmptyExample(t *testing.T) {
	for _, in := range []string{"", "   \n\n", "# only comments\n"} {
		if _, err := newImportApp().AnalyseEnvImport(in, "", "", ""); err == nil {
			t.Errorf("AnalyseEnvImport(%q) succeeded, want an error", in)
		}
	}
}

func TestCreateVaultFromImport(t *testing.T) {
	app := newImportApp()

	entries := []ImportEntry{
		{Key: "POSTGRES_HOST", GroupPrefix: "POSTGRES_", ValueDev: "localhost", ValueProd: "db.internal"},
		{Key: "POSTGRES_PORT", GroupPrefix: "POSTGRES_", ValueDev: "5432"},
		{Key: "DEBUG", Type: "env", ValueDev: "true"},
	}

	v, err := app.CreateVaultFromImport("CineVault", "aus .env.example", "", entries)
	if err != nil {
		t.Fatalf("CreateVaultFromImport: %v", err)
	}

	if v.Name != "CineVault" || v.Description != "aus .env.example" {
		t.Errorf("metadata not set: %+v", v)
	}
	if len(v.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(v.Entries))
	}

	first := v.Entries[0]
	if first.Key != "POSTGRES_HOST" || first.GroupPrefix != "POSTGRES_" {
		t.Errorf("grouping not carried over: %+v", first)
	}
	if first.ValueDev != "localhost" || first.ValueProd != "db.internal" {
		t.Errorf("values not carried over: %+v", first)
	}
	if first.Type != vault.EntryTypeSecret {
		t.Errorf("type = %q, want secret by default", first.Type)
	}
	if v.Entries[2].Type != vault.EntryTypeEnv {
		t.Errorf("an explicit type was overridden: %+v", v.Entries[2])
	}

	// And it must be in the service, not just returned.
	if stored := app.ListVaults(); len(stored) != 1 || len(stored[0].Entries) != 3 {
		t.Errorf("vault not stored properly: %+v", stored)
	}
}

func TestCreateVaultFromImportRejectsEmptyInput(t *testing.T) {
	app := newImportApp()

	if _, err := app.CreateVaultFromImport("", "", "", []ImportEntry{{Key: "A"}}); err == nil {
		t.Error("a vault without a name was accepted")
	}
	if _, err := app.CreateVaultFromImport("Name", "", "", nil); err == nil {
		t.Error("a vault without entries was accepted")
	}
	if got := app.ListVaults(); len(got) != 0 {
		t.Errorf("a rejected import left %d vaults behind", len(got))
	}
}

func TestCreateVaultFromImportRollsBackWhenPersistFails(t *testing.T) {
	app := newImportApp()
	app.loadErr = errWriteBlocked

	_, err := app.CreateVaultFromImport("CineVault", "", "", []ImportEntry{{Key: "A", ValueDev: "1"}})
	if err == nil {
		t.Fatal("import succeeded although persistence is blocked")
	}
	if got := app.ListVaults(); len(got) != 0 {
		t.Errorf("the failed import left %d vaults in memory", len(got))
	}
}
