package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecopyFiles(t *testing.T) {
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

	tenant := "tenantRecopy"
	file := filepath.Join(os.TempDir(), "recopy.txt")
	destDir := filepath.Join(os.TempDir(), "recopy-dest")
	os.MkdirAll(destDir, 0755)
	os.WriteFile(file, []byte("recopy"), 0644)
	defer os.Remove(file)
	defer os.RemoveAll(destDir)
	if err := markProcessed(db, tenant, file, 6, destDir); err != nil {
		t.Fatalf("erro ao marcar processado: %v", err)
	}
	var id int
	db.QueryRow("SELECT id FROM processed_files WHERE tenant=? AND file=?", tenant, file).Scan(&id)
	if id == 0 {
		t.Fatalf("id não encontrado")
	}
	os.Remove(filepath.Join(destDir, filepath.Base(file))) // garante que não existe
	if err := recopyFiles(db, tenant, []int{id}); err != nil {
		t.Fatalf("erro ao recopy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, filepath.Base(file))); err != nil {
		t.Errorf("arquivo não recopiado")
	}
}

func TestCopyFilePermissionDenied(t *testing.T) {
	srcFile, err := os.CreateTemp("", "src-perm-*.txt")
	if err != nil {
		t.Fatalf("erro ao criar src: %v", err)
	}
	defer os.Remove(srcFile.Name())
	srcFile.Write([]byte("fail"))
	srcFile.Close()
	destDir := filepath.Join(os.TempDir(), "dest-perm")
	os.MkdirAll(destDir, 0000) // sem permissão
	defer os.Chmod(destDir, 0755)
	defer os.RemoveAll(destDir)
	destFile := filepath.Join(destDir, "fail.txt")
	err = copyFile(srcFile.Name(), destFile)
	if err == nil {
		t.Errorf("esperado erro de permissão, mas não ocorreu")
	}
}
