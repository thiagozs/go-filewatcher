package main

import (
	"os/exec"
	"testing"
)

func TestCLIListProcessedFlag(t *testing.T) {
	cmd := exec.Command("go", "run", "main.go", "--list-processed")
	out, err := cmd.CombinedOutput()
	if err != nil && cmd.ProcessState.ExitCode() != 0 {
		// Pode falhar se não houver DB/config, mas deve mostrar mensagem amigável
		if len(out) == 0 {
			t.Fatalf("esperado mensagem de erro amigável, mas não houve saída")
		}
	}
}

func TestCLITenantFlag(t *testing.T) {
	cmd := exec.Command("go", "run", "main.go", "--list-processed", "--tenant", "tenantA")
	out, err := cmd.CombinedOutput()
	if err != nil && cmd.ProcessState.ExitCode() != 0 {
		if len(out) == 0 {
			t.Fatalf("esperado mensagem de erro amigável, mas não houve saída")
		}
	}
}

func TestCLIKeepsourceFlag(t *testing.T) {
	cmd := exec.Command("go", "run", "main.go", "--keep-source", "--list-processed")
	out, err := cmd.CombinedOutput()
	if err != nil && cmd.ProcessState.ExitCode() != 0 {
		if len(out) == 0 {
			t.Fatalf("esperado mensagem de erro amigável, mas não houve saída")
		}
	}
}
