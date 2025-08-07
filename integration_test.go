package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWatcher_BasicFlow(t *testing.T) {
	dirWatch, err := os.MkdirTemp("", "watch-")
	if err != nil {
		t.Fatalf("erro ao criar dir watch: %v", err)
	}
	defer os.RemoveAll(dirWatch)
	dirDest, err := os.MkdirTemp("", "dest-")
	if err != nil {
		t.Fatalf("erro ao criar dir dest: %v", err)
	}
	defer os.RemoveAll(dirDest)

	configContent := []byte(`tenants:
  - name: test
    watch_dir: "` + dirWatch + `"
    dest_dir: "` + dirDest + `"
`)
	configFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("erro ao criar config: %v", err)
	}
	defer os.Remove(configFile.Name())
	if _, err := configFile.Write(configContent); err != nil {
		t.Fatalf("erro ao escrever config: %v", err)
	}
	configFile.Close()

	dbFile, err := os.CreateTemp("", "filewatcher-*.db")
	if err != nil {
		t.Fatalf("erro ao criar db: %v", err)
	}
	os.Remove(dbFile.Name()) // será criado pelo initDB
	defer os.Remove(dbFile.Name())

	os.Rename(configFile.Name(), "config.yaml")
	defer os.Remove("config.yaml")

	db, err := initDB()
	if err != nil {
		t.Fatalf("erro ao iniciar db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go watchTenant(ctx, TenantConfig{
		Name:     "test",
		WatchDir: dirWatch,
		DestDir:  dirDest,
	}, db, &wg, false)

	// Pequeno delay para garantir que o watcher está ativo
	time.Sleep(500 * time.Millisecond)

	// Cria arquivo na pasta monitorada
	fileName := filepath.Join(dirWatch, "arquivo.txt")
	conteudo := []byte("conteudo de teste")
	if err := ioutil.WriteFile(fileName, conteudo, 0644); err != nil {
		cancel()
		wg.Wait()
		t.Fatalf("erro ao criar arquivo: %v", err)
	}

	// Aguarda processamento (até 10s)
	destFile := filepath.Join(dirDest, "arquivo.txt")
	ok := false
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(destFile); err == nil {
			ok = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	cancel()
	wg.Wait()

	if !ok {
		t.Fatalf("arquivo não copiado para destino: %s", destFile)
	}
	data, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("erro ao ler arquivo destino: %v", err)
	}
	if string(data) != string(conteudo) {
		t.Errorf("conteúdo do arquivo destino incorreto: %s", string(data))
	}
}
