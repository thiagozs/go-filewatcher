# go-filewatcher

## Descrição Geral

O **go-filewatcher** é uma aplicação escrita em Go para monitoramento de diretórios, cópia e controle de arquivos processados, com suporte a múltiplos tenants (clientes/ambientes isolados). O sistema é altamente configurável via arquivo YAML e mantém um banco de dados SQLite para rastreamento dos arquivos já processados.

---

## Funcionalidades Principais

- **Monitoramento de Diretórios**: Observa diretórios configurados para cada tenant e detecta novos arquivos criados.
- **Cópia Automática**: Ao detectar um novo arquivo, realiza a cópia para o diretório de destino correspondente ao tenant.
- **Controle de Processamento**: Registra no banco de dados os arquivos já processados, evitando duplicidade.
- **Remoção Opcional do Arquivo Original**: Por padrão, remove o arquivo original após a cópia, mas pode manter o arquivo com a flag `--keep-source`.
- **Listagem de Arquivos Processados**: Permite listar arquivos já processados, com paginação e filtro por tenant.
- **Recópia de Arquivos**: Possibilita recopiar arquivos processados para o destino, a partir do ID registrado no banco.
- **Exclusão de Arquivos Processados**: Permite remover registros e arquivos do disco, a partir do ID.
- **Shutdown Graceful**: Suporte a encerramento seguro via sinais do sistema.

---

## Estrutura de Configuração

A configuração é feita via arquivo `config.yaml` (exemplo em `config.example.yaml`):

```yaml
tenants:
	- name: tenantA
		watch_dir: "/tmp/tenantA/incoming"
		dest_dir: "/tmp/tenantA/outgoing"
	- name: tenantB
		watch_dir: "/tmp/tenantB/incoming"
		dest_dir: "/tmp/tenantB/outgoing"
```

Cada tenant possui:

- `name`: Nome identificador do tenant
- `watch_dir`: Diretório a ser monitorado
- `dest_dir`: Diretório de destino para cópia dos arquivos

---

## Banco de Dados

- Utiliza SQLite (`filewatcher.db`)
- Tabela principal: `processed_files`
  - Campos: id, tenant, file, processed_at, file_size, dest_dir
  - Garante unicidade por tenant e arquivo

---

## Uso e Execução

### Execução padrão

```sh
go run main.go
```

---

## Comandos do Makefile

O projeto possui um Makefile para facilitar a compilação e execução:

| Comando         | Descrição                                 |
|-----------------|-------------------------------------------|
| make build      | Compila o binário em `bin/go-filewatcher` |
| make run        | Compila e executa o binário               |
| make clean      | Remove a pasta `bin/` e o binário gerado  |

Exemplo de uso:

```sh
make build
make run
make clean
```

### Principais flags

- `--list-processed` : Lista arquivos processados
- `--tenant <nome>` : Filtra operações por tenant
- `--keep-source` ou `-k` : Mantém o arquivo original após cópia
- `--delete-processed <ids>` : Remove arquivos processados por IDs (requer --tenant)
- `--recopy <ids>` : Recopia arquivos processados por IDs (requer --tenant)
- `--page <n>` : Página da listagem (default 1)
- `--page-size <n>` : Tamanho da página (default 20)

Exemplo de listagem:

```sh
go run main.go --list-processed --tenant tenantA --page 1 --page-size 10
```

Exemplo de recópia:

```sh
go run main.go --recopy 1,2,3 --tenant tenantA
```

Exemplo de exclusão:

```sh
go run main.go --delete-processed 4,5 --tenant tenantB
```

---

## Dependências

- [fsnotify](https://github.com/fsnotify/fsnotify) — Monitoramento de arquivos
- [tablewriter](https://github.com/olekukonko/tablewriter) — Exibição de tabelas no terminal
- [yaml.v2](https://gopkg.in/yaml.v2) — Leitura de arquivos YAML
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — Banco de dados SQLite

---

## Observações

- O projeto suporta múltiplos tenants simultaneamente.
- O arquivo de configuração deve estar no mesmo diretório do executável, nomeado como `config.yaml`.
- O banco de dados é criado automaticamente na primeira execução.
- O sistema é tolerante a falhas e realiza shutdown seguro ao receber sinais SIGINT/SIGTERM.

---

## Autor

- Thiago Zilli Sarmento(github.com/thiagozs)
