package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// Testa a sincronização automática entre diretórios e banco de dados
func TestSyncTenantDirs(t *testing.T) {
	dbFile, err := os.CreateTemp("", "filewatcher-sync-*.db")
	if err != nil {
		t.Fatalf("erro ao criar db: %v", err)
	}
	dbFileName := dbFile.Name()
	dbFile.Close()
	os.Remove(dbFileName)
	defer os.Remove(dbFileName)

	db, err := sql.Open("sqlite", dbFileName)
	if err != nil {
		t.Fatalf("erro ao iniciar db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS processed_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant TEXT,
		file TEXT,
		processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		file_size INTEGER,
		dest_dir TEXT,
		UNIQUE(tenant, file)
	);`)
	if err != nil {
		t.Fatalf("erro ao criar tabela: %v", err)
	}

	watchDir := t.TempDir()
	destDir := t.TempDir()
	fileName := "syncfile.txt"
	watchFile := filepath.Join(watchDir, fileName)
	destFile := filepath.Join(destDir, fileName)
	os.WriteFile(watchFile, []byte("conteudo"), 0644)

	tenant := TenantConfig{
		Name:     "tenantSync",
		WatchDir: watchDir,
		DestDir:  destDir,
	}

	// Executa sync
	err = syncTenantDirs(db, tenant)
	if err != nil {
		t.Fatalf("erro no syncTenantDirs: %v", err)
	}

	// Deve ter copiado para o destino
	if _, err := os.Stat(destFile); err != nil {
		t.Errorf("arquivo não copiado para destino")
	}
	// Deve estar registrado no banco
	processed, err := hasProcessed(db, tenant.Name, watchFile)
	if err != nil {
		t.Fatalf("erro ao checar hasProcessed: %v", err)
	}
	if !processed {
		t.Errorf("arquivo não registrado no banco após sync")
	}
}
