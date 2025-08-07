package main

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("erro ao criar temp: %v", err)
	}
	defer os.Remove(f.Name())

	configContent := `tenants:
  - name: test
    watch_dir: /tmp/test/in
    dest_dir: /tmp/test/out
`
	if _, err := f.WriteString(configContent); err != nil {
		t.Fatalf("erro ao escrever config: %v", err)
	}
	f.Close()

	cfg, err := loadConfig(f.Name())
	if err != nil {
		t.Fatalf("erro ao carregar config: %v", err)
	}
	if len(cfg.Tenants) != 1 {
		t.Errorf("esperado 1 tenant, veio %d", len(cfg.Tenants))
	}
	if cfg.Tenants[0].Name != "test" {
		t.Errorf("nome do tenant incorreto: %s", cfg.Tenants[0].Name)
	}
}
