package model

import "fmt"

// migrateLocalModels runs the AutoMigrate of the tables that exist only in
// this fork. Keeping the list here leaves model/main.go identical to upstream
// except for the single call to this function.
func migrateLocalModels() error {
	return DB.AutoMigrate(
		&QuotaCycle{},
		&QuotaPlan{},
		&QuotaItem{},
		&TemporaryQuotaRequest{},
	)
}

// MigrateLocalOptions upgrades fork-only option rows before the master node
// serves. main.go calls it during InitResources on the master node.
func MigrateLocalOptions() error {
	if err := MigrateChatPresetsOption(); err != nil {
		return fmt.Errorf("chat presets: %w", err)
	}
	return nil
}
