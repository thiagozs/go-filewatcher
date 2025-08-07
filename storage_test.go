package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMarkAndHasProcessed(t *testing.T) {
	dbFile, err := os.CreateTemp("", "filewatcher-*.db")
	if err != nil {
		t.Fatalf("erro ao criar db: %v", err)
	}
	dbFileName := dbFile.Name()
	dbFile.Close()
	os.Remove(dbFileName) // garante que não existe
	defer os.Remove(dbFileName)

	db, err := sql.Open("sqlite", dbFileName)
	if err != nil {
		t.Fatalf("erro ao iniciar db: %v", err)
	}
	defer db.Close()

	// Cria a tabela manualmente para garantir isolamento
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

	tenant := "tenantTest"
	file := "/tmp/testfile.txt"
	processed, err := hasProcessed(db, tenant, file)
	if err != nil {
		t.Fatalf("erro ao checar hasProcessed: %v", err)
	}
	if processed {
		t.Errorf("esperado não processado")
	}
	if err := markProcessed(db, tenant, file, 123, "/tmp/dest"); err != nil {
		t.Fatalf("erro ao marcar processado: %v", err)
	}
	processed, err = hasProcessed(db, tenant, file)
	if err != nil {
		t.Fatalf("erro ao checar hasProcessed: %v", err)
	}
	if !processed {
		t.Errorf("esperado processado")
	}
}

func TestCopyFile(t *testing.T) {
	srcFile, err := os.CreateTemp("", "src-*.txt")
	if err != nil {
		t.Fatalf("erro ao criar src: %v", err)
	}
	defer os.Remove(srcFile.Name())
	conteudo := []byte("abc123")
	srcFile.Write(conteudo)
	srcFile.Close()
	destFile := filepath.Join(os.TempDir(), "dest-abc123.txt")
	defer os.Remove(destFile)
	if err := copyFile(srcFile.Name(), destFile); err != nil {
		t.Fatalf("erro ao copiar arquivo: %v", err)
	}
	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("erro ao ler destino: %v", err)
	}
	if string(data) != string(conteudo) {
		t.Errorf("conteúdo incorreto: %s", string(data))
	}
}
