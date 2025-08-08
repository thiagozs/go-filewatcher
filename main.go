package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v2"
	_ "modernc.org/sqlite"
)

type TenantConfig struct {
	Name     string `yaml:"name"`
	WatchDir string `yaml:"watch_dir"`
	DestDir  string `yaml:"dest_dir"`
}

type Config struct {
	Tenants []TenantConfig `yaml:"tenants"`
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt interface{}
		rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		if name == column {
			return true, nil
		}
	}
	return false, nil
}

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "./filewatcher.db")
	if err != nil {
		return nil, err
	}
	// Cria tabela se não existir
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS processed_files (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            tenant TEXT,
            file TEXT,
            processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            file_size INTEGER,
            dest_dir TEXT,
            UNIQUE(tenant, file)
        );
    `)
	if err != nil {
		return nil, err
	}

	// Adiciona colunas só se não existem
	if ok, _ := columnExists(db, "processed_files", "file_size"); !ok {
		db.Exec(`ALTER TABLE processed_files ADD COLUMN file_size INTEGER`)
	}

	if ok, _ := columnExists(db, "processed_files", "dest_dir"); !ok {
		db.Exec(`ALTER TABLE processed_files ADD COLUMN dest_dir TEXT`)
	}
	// Rename de coluna não incluso por ser mais complexo em SQLite
	return db, nil
}

func hasProcessed(db *sql.DB, tenant, file string) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(1) FROM processed_files WHERE tenant=? AND file=?", tenant, file).Scan(&count)
	return count > 0, err
}

func markProcessed(db *sql.DB, tenant, file string, fileSize int64, destDir string) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO processed_files(tenant, file, file_size, dest_dir) VALUES (?, ?, ?, ?)",
		tenant, file, fileSize, destDir,
	)
	return err
}

func truncateFileName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	return name[:max-3] + "..."
}

func normalizePath(path string, max int) string {
	if len(path) <= max {
		return path
	}
	return "..." + path[len(path)-max+3:]
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func parseIDs(csv string) ([]int, error) {
	var ids []int
	for _, part := range filepath.SplitList(csv) {
		for _, idStr := range splitByCommaOrWhitespace(part) {
			if idStr == "" {
				continue
			}
			var id int
			_, err := fmt.Sscanf(idStr, "%d", &id)
			if err != nil {
				return nil, fmt.Errorf("invalid ID: %q", idStr)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func splitByCommaOrWhitespace(s string) []string {
	return filepath.SplitList(strings.ReplaceAll(s, ",", string(os.PathListSeparator)))
}

func recopyFiles(db *sql.DB, tenant string, ids []int) error {
	if tenant == "" {
		return fmt.Errorf("tenant must be specified for recopy")
	}
	for _, id := range ids {
		var filePath, destDir string
		var fileSize sql.NullInt64
		err := db.QueryRow("SELECT file, dest_dir, file_size FROM processed_files WHERE id = ? AND tenant = ?", id, tenant).
			Scan(&filePath, &destDir, &fileSize)
		if err != nil {
			log.Printf("[Recopy] Failed to find file with id %d: %v", id, err)
			continue
		}
		destFile := filepath.Join(destDir, filepath.Base(filePath))
		err = copyFile(filePath, destFile)
		if err != nil {
			log.Printf("[Recopy] Failed to copy file id %d: %v", id, err)
		} else {
			log.Printf("[Recopy] Copied file id %d: %s -> %s", id, filePath, destFile)
		}
	}
	return nil
}

func deleteProcessedFiles(db *sql.DB, tenant string, ids []int) error {
	if tenant == "" {
		return fmt.Errorf("tenant must be specified for delete-processed")
	}
	for _, id := range ids {
		var file, destDir string
		err := db.QueryRow("SELECT file,dest_dir FROM processed_files WHERE id = ? AND tenant = ?", id, tenant).Scan(&file, &destDir)
		if err != nil {
			log.Printf("[Delete] Failed to find file with id %d: %v", id, err)
			continue
		}

		_, err = db.Exec("DELETE FROM processed_files WHERE id = ? AND tenant = ?", id, tenant)
		if err != nil {
			log.Printf("[Delete] Failed to delete DB entry id %d: %v", id, err)
		} else {
			filePath := filepath.Join(destDir, filepath.Base(file))
			if err := os.Remove(filePath); err != nil {
				log.Printf("[Delete] Deleted DB entry id %d (file: %s), but failed to remove file from disk: %v", id, filePath, err)
			} else {
				log.Printf("[Delete] Deleted DB entry id %d (file: %s) and removed file from disk.", id, filePath)
			}
		}
	}
	return nil
}

func listProcessedFiles(db *sql.DB, tenant string, page int, pageSize int) error {
	var rows *sql.Rows
	var err error
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	query := "SELECT id, tenant, file, processed_at, file_size, dest_dir FROM processed_files "
	var args []interface{}
	if tenant != "" {
		query += "WHERE tenant = ? "
		args = append(args, tenant)
	}
	query += "ORDER BY processed_at DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	rows, err = db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "Tenant", "File", "Size", "Dest Dir", "Processed At"})

	for rows.Next() {
		var id int
		var tenantName, file, processedAt string
		var fileSize sql.NullInt64
		var destDir sql.NullString
		if err := rows.Scan(&id, &tenantName, &file, &processedAt, &fileSize, &destDir); err != nil {
			return err
		}
		fileDisplay := truncateFileName(filepath.Base(file), 40)
		destDisplay := ""
		if destDir.Valid {
			destDisplay = normalizePath(destDir.String, 28)
		}
		sizeDisplay := ""
		if fileSize.Valid {
			sizeDisplay = humanSize(fileSize.Int64)
		}
		table.Append([]string{
			fmt.Sprintf("%d", id),
			tenantName,
			fileDisplay,
			sizeDisplay,
			destDisplay,
			processedAt,
		})
	}
	table.Render()
	fmt.Printf("Page %d (Page Size %d)\n", page, pageSize)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func waitFileStable(filename string, stableFor time.Duration, maxWait time.Duration) error {
	const poll = 1 * time.Second
	waited := time.Duration(0)
	var lastSize int64 = -1
	stable := time.Duration(0)

	for waited < maxWait {
		fi, err := os.Stat(filename)
		if err != nil {
			return err
		}
		size := fi.Size()
		if size == lastSize {
			stable += poll
			if stable >= stableFor {
				return nil // Stable!
			}
		} else {
			stable = 0
			lastSize = size
		}
		time.Sleep(poll)
		waited += poll
	}
	return fmt.Errorf("file %s did not stabilize within %v", filename, maxWait)
}

// Agora recebe context.Context para shutdown graceful!
func watchTenant(ctx context.Context, tc TenantConfig, db *sql.DB, wg *sync.WaitGroup, keepSource bool) {
	defer wg.Done()
	if _, err := os.Stat(tc.WatchDir); os.IsNotExist(err) {
		log.Printf("[%s] Watch dir does not exist: %s", tc.Name, tc.WatchDir)
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[%s] Failed to create watcher: %v", tc.Name, err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(tc.WatchDir); err != nil {
		log.Printf("[%s] Failed to add directory: %v", tc.Name, err)
		return
	}
	log.Printf("[%s] Watching: %s", tc.Name, tc.WatchDir)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[%s] Shutdown requested, watcher exiting.", tc.Name)
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				fi, err := os.Stat(event.Name)
				if err == nil && !fi.IsDir() {
					processed, err := hasProcessed(db, tc.Name, event.Name)
					if err != nil {
						log.Printf("[%s] Error checking persistence: %v", tc.Name, err)
						continue
					}
					if processed {
						log.Printf("[%s] File %s already processed. Skipping.", tc.Name, event.Name)
						continue
					}
					if err := waitFileStable(event.Name, 3*time.Second, 2*time.Minute); err != nil {
						log.Printf("[%s] File %s did not stabilize: %v", tc.Name, event.Name, err)
						continue
					}
					destFile := filepath.Join(tc.DestDir, filepath.Base(event.Name))
					err = copyFile(event.Name, destFile)
					if err != nil {
						log.Printf("[%s] Failed to copy %s: %v", tc.Name, event.Name, err)
					} else {
						log.Printf("[%s] Copied %s -> %s", tc.Name, event.Name, destFile)
						fi, _ := os.Stat(event.Name)
						if err := markProcessed(db, tc.Name, event.Name, fi.Size(), tc.DestDir); err != nil {
							log.Printf("[%s] Failed to mark file as processed: %v", tc.Name, err)
						}
						if !keepSource {
							if err := os.Remove(event.Name); err != nil {
								log.Printf("[%s] Failed to remove original file %s: %v", tc.Name, event.Name, err)
							} else {
								log.Printf("[%s] Removed original file %s", tc.Name, event.Name)
							}
						} else {
							log.Printf("[%s] Source file kept as per --keep-source flag: %s", tc.Name, event.Name)
						}
					}
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[%s] Watcher error: %v", tc.Name, err)
		}
	}
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// Sincroniza arquivos entre diretórios e banco ao iniciar
func syncTenantDirs(db *sql.DB, tc TenantConfig) error {
	watchFiles, _ := os.ReadDir(tc.WatchDir)
	destFiles, _ := os.ReadDir(tc.DestDir)
	filesSet := make(map[string]struct{})
	// Indexa todos os arquivos dos dois diretórios
	for _, f := range watchFiles {
		if !f.IsDir() {
			filesSet[f.Name()] = struct{}{}
		}
	}
	for _, f := range destFiles {
		if !f.IsDir() {
			filesSet[f.Name()] = struct{}{}
		}
	}
	for fname := range filesSet {
		srcPath := filepath.Join(tc.WatchDir, fname)
		dstPath := filepath.Join(tc.DestDir, fname)
		srcExists := fileExists(srcPath)
		dstExists := fileExists(dstPath)
		// Se não está no banco, processa
		processed, _ := hasProcessed(db, tc.Name, srcPath)
		if !processed {
			// Se só existe no destino, registra no banco
			if !srcExists && dstExists {
				fi, _ := os.Stat(dstPath)
				markProcessed(db, tc.Name, srcPath, fi.Size(), tc.DestDir)
				log.Printf("[Sync] Only in dest: Registering file '%s' in database for tenant '%s'", dstPath, tc.Name)
			}
			// Se só existe no watch, copia e registra
			if srcExists && !dstExists {
				err := copyFile(srcPath, dstPath)
				if err != nil {
					log.Printf("[Sync] Error copying '%s' to '%s': %v", srcPath, dstPath, err)
				} else {
					log.Printf("[Sync] Only in watch: Copied '%s' to '%s' for tenant '%s'", srcPath, dstPath, tc.Name)
				}
				fi, _ := os.Stat(srcPath)
				markProcessed(db, tc.Name, srcPath, fi.Size(), tc.DestDir)
			}
			// Se existe nos dois, só registra
			if srcExists && dstExists {
				fi, _ := os.Stat(srcPath)
				markProcessed(db, tc.Name, srcPath, fi.Size(), tc.DestDir)
				log.Printf("[Sync] In both: Registering file '%s' in database for tenant '%s'", srcPath, tc.Name)
			}
		}
	}
	return nil
}

func main() {

	var cfg *Config
	var err error
	cfg, err = loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	installServiceFlag := flag.Bool("install-service", false, "Instala o serviço systemd para inicialização automática")
	listFlag := flag.Bool("list-processed", false, "List processed files from the database and exit")
	tenantFlag := flag.String("tenant", "", "Filter processed files by tenant name (use with --list-processed)")
	keepSourceFlag := flag.Bool("keep-source", false, "Keep the source file after copying (do not delete original)")
	flag.BoolVar(keepSourceFlag, "k", false, "Keep the source file after copying (do not delete original)")
	deleteProcessedFlag := flag.String("delete-processed", "", "Delete processed files by comma-separated IDs (use with --tenant)")
	recopyFlag := flag.String("recopy", "", "Recopy processed files by comma-separated IDs (use with --tenant)")
	pageFlag := flag.Int("page", 1, "Page number for processed files listing (default 1)")
	pageSizeFlag := flag.Int("page-size", 20, "Number of records per page (default 20)")

	flag.Parse()

	if *installServiceFlag {
		exePath, err := os.Executable()
		if err != nil {
			log.Fatalf("Erro ao obter path do binário: %v", err)
		}
		workDir, err := os.Getwd()
		if err != nil {
			log.Fatalf("Erro ao obter diretório de trabalho: %v", err)
		}
		currentUser, err := user.Current()
		if err != nil {
			log.Fatalf("Erro ao obter usuário: %v", err)
		}
		serviceContent := fmt.Sprintf(`[Unit]
Description=GFW Service
After=network.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=on-failure
User=%s

[Install]
WantedBy=multi-user.target
`, exePath, workDir, currentUser.Username)
		tmpService := "gfw.service"
		if err := os.WriteFile(tmpService, []byte(serviceContent), 0644); err != nil {
			log.Fatalf("Erro ao criar arquivo de serviço temporário: %v", err)
		}
		defer os.Remove(tmpService)
		cmdCopy := exec.Command("sudo", "cp", tmpService, "/etc/systemd/system/gfw.service")
		cmdCopy.Stdout = os.Stdout
		cmdCopy.Stderr = os.Stderr
		if err := cmdCopy.Run(); err != nil {
			log.Fatalf("Erro ao copiar serviço para /etc/systemd/system: %v", err)
		}
		exec.Command("sudo", "systemctl", "daemon-reload").Run()
		exec.Command("sudo", "systemctl", "enable", "--now", "gfw.service").Run()
		fmt.Println("Serviço systemd instalado e iniciado com sucesso!")

		return
	}

	db, err := initDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Sincroniza arquivos antes de iniciar watchers
	for _, tenant := range cfg.Tenants {
		if err := syncTenantDirs(db, tenant); err != nil {
			log.Printf("[Sync] Error syncing tenant %s: %v", tenant.Name, err)
		}
	}

	if *deleteProcessedFlag != "" {
		ids, err := parseIDs(*deleteProcessedFlag)
		if err != nil {
			log.Fatalf("Failed to parse delete-processed IDs: %v", err)
		}
		if err := deleteProcessedFiles(db, *tenantFlag, ids); err != nil {
			log.Fatalf("Failed to delete processed files: %v", err)
		}
		return
	}

	if *recopyFlag != "" {
		ids, err := parseIDs(*recopyFlag)
		if err != nil {
			log.Fatalf("Failed to parse recopy IDs: %v", err)
		}
		if err := recopyFiles(db, *tenantFlag, ids); err != nil {
			log.Fatalf("Failed to recopy files: %v", err)
		}
		return
	}

	if *listFlag {
		if err := listProcessedFiles(db, *tenantFlag, *pageFlag, *pageSizeFlag); err != nil {
			log.Fatalf("Failed to list processed files: %v", err)
		}
		return
	}

	// Graceful shutdown: Context + signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Println("Graceful shutdown signal received.")
		cancel()
	}()

	var wg sync.WaitGroup
	for _, tenant := range cfg.Tenants {
		wg.Add(1)
		go watchTenant(ctx, tenant, db, &wg, *keepSourceFlag)
	}
	log.Println("Filewatcher started and running.")
	wg.Wait()
}
