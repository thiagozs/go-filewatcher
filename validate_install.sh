#!/bin/bash
# Validação automática da instalação do serviço go-filewatcher

set -e

SERVICE=gfw.service
BIN=bin/gfw

# 1. Compila o binário
make build

# 2. Garante config.yaml e banco no diretório do serviço
if [ ! -f config.yaml ]; then
  echo "[ERRO] config.yaml não encontrado no diretório atual. Copie ou crie antes de instalar o serviço."
  exit 1
fi
if [ ! -f filewatcher.db ]; then
  echo "[INFO] Criando banco de dados inicial (filewatcher.db)"
  touch filewatcher.db
fi

# 3. Instala o serviço
sudo ./$BIN --install-service

# 4. Verifica status
systemctl is-active --quiet $SERVICE && echo "[OK] Serviço ativo" || { echo "[ERRO] Serviço não está ativo"; exit 1; }

# 5. Mostra logs recentes
journalctl -u $SERVICE -n 10 --no-pager

echo "Validação concluída com sucesso!"
