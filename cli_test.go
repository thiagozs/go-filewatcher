package main

import (
	"os"
	"testing"
	"path/filepath"
)

func TestMultiTenants(t *testing.T) {
	dbFile, err := os.CreateTemp("", "filewatcher-*.db")
	if err != nil {
		t.Fatalf("erro ao criar db: %v", err)
	}
	os.Remove(dbFile.Name())
	defer os.Remove(dbFile.Name())

	db, err := initDB()
	if err != nil {
		t.Fatalf("erro ao iniciar db: %v", err)
	}
	defer db.Close()

	tenantA := "tenantA"
	tenantB := "tenantB"
	file := "/tmp/testfile.txt"
	if err := markProcessed(db, tenantA, file, 100, "/tmp/destA"); err != nil {
		t.Fatalf("erro ao marcar processado tenantA: %v", err)
	}
	if err := markProcessed(db, tenantB, file, 200, "/tmp/destB"); err != nil {
		t.Fatalf("erro ao marcar processado tenantB: %v", err)
	}
	procA, _ := hasProcessed(db, tenantA, file)
	procB, _ := hasProcessed(db, tenantB, file)
	if !procA || !procB {
		t.Errorf("esperado processado para ambos tenants")
	}
}

func TestDeleteProcessedFiles(t *testing.T) {
	dbFile, err := os.CreateTemp("", "filewatcher-*.db")
	if err != nil {
		t.Fatalf("erro ao criar db: %v", err)
	}
	os.Remove(dbFile.Name())
	defer os.Remove(dbFile.Name())

	db, err := initDB()
	if err != nil {
		t.Fatalf("erro ao iniciar db: %v", err)
	}
	defer db.Close()

	tenant := "tenantDel"
	file := "/tmp/testfileDel.txt"
	if err := markProcessed(db, tenant, file, 123, "/tmp/destDel"); err != nil {
		t.Fatalf("erro ao marcar processado: %v", err)
	}
	var id int
	db.QueryRow("SELECT id FROM processed_files WHERE tenant=? AND file=?", tenant, file).Scan(&id)
	if id == 0 {
		t.Fatalf("id não encontrado")
	}
	// Cria arquivo fake no destino
	destFile := filepath.Join("/tmp/destDel", filepath.Base(file))
	os.MkdirAll("/tmp/destDel", 0755)
	os.WriteFile(destFile, []byte("abc"), 0644)
	defer os.Remove(destFile)
	if err := deleteProcessedFiles(db, tenant, []int{id}); err != nil {
		t.Fatalf("erro ao deletar processado: %v", err)
	}
	proc, _ := hasProcessed(db, tenant, file)
	if proc {
		t.Errorf("esperado não processado após delete")
	}
	if _, err := os.Stat(destFile); !os.IsNotExist(err) {
		t.Errorf("arquivo destino não removido")
	}
}
