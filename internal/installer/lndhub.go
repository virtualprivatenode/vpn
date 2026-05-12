// internal/installer/lndhub.go

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ripsline/virtual-private-node/internal/config"
	"github.com/ripsline/virtual-private-node/internal/logger"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/system"
)

const (
	lndhubVersion = "1.0.2"
	lndhubRepo    = "https://github.com/getAlby/lndhub.go.git"
	goVersion     = "1.26.0"
	goTarball     = "go1.26.0.linux-amd64.tar.gz"
	goDownloadURL = "https://go.dev/dl/go1.26.0.linux-amd64.tar.gz"
	goSHA256      = "aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235" // from https://go.dev/dl/
	goInstallDir  = "/usr/local/go"
)

// loginPattern matches LndHub login strings: alphanumeric, 1-40 chars.
var loginPattern = regexp.MustCompile(`^[a-zA-Z0-9]{1,40}$`)

func LndHubVersionStr() string { return lndhubVersion }

// validateLogin ensures a login string is safe for use in database queries.
// LndHub generates alphanumeric login strings. Anything else is rejected.
func validateLogin(login string) error {
	if !loginPattern.MatchString(login) {
		return fmt.Errorf("invalid login format: must be alphanumeric, got %q", login)
	}
	return nil
}

// ── Go toolchain ─────────────────────────────────────────

func installGoToolchain() error {
	goPath := goInstallDir + "/bin/go"
	if _, err := os.Stat(goPath); err == nil {
		output, err := system.RunContext(5*time.Second, goPath, "version")
		if err == nil && output != "" {
			logger.Install("Go already installed: %s", output)
			return nil
		}
	}

	tarball := "/tmp/" + goTarball
	if err := system.DownloadRequireTor(goDownloadURL, tarball); err != nil {
		return fmt.Errorf("download Go: %w", err)
	}
	defer os.Remove(tarball)

	// Verify SHA256 checksum of Go tarball
	output, err := system.RunOutput("sha256sum", tarball)
	if err != nil {
		return fmt.Errorf("checksum Go tarball: %w", err)
	}
	if !strings.HasPrefix(output, goSHA256) {
		return fmt.Errorf("Go tarball checksum mismatch: got %s", strings.Fields(output)[0])
	}
	logger.Install("Go tarball checksum verified")

	system.SudoRunSilent("rm", "-rf", goInstallDir)

	if err := system.SudoRun("tar", "-C", "/usr/local", "-xzf", tarball); err != nil {
		return fmt.Errorf("extract Go: %w", err)
	}

	logger.Install("Go toolchain installed: %s", goVersion)
	return nil
}

// ── PostgreSQL ───────────────────────────────────────────

func installPostgreSQL() error {
	if err := system.SudoRun("apt-get", "install", "-y", "-qq",
		"postgresql", "postgresql-client"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "postgresql"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "start", "postgresql")
}

func createLndHubDatabase(dbPassword string) error {
	checkOutput, err := system.SudoRunOutput("-u", "postgres", "psql", "-tAc",
		"SELECT 1 FROM pg_roles WHERE rolname='lndhub'")
	if err == nil && strings.TrimSpace(checkOutput) == "1" {
		logger.Install("PostgreSQL user lndhub already exists")
		return nil
	}

	createUser := fmt.Sprintf("CREATE USER lndhub WITH PASSWORD '%s'", dbPassword)
	if err := system.SudoRun("-u", "postgres", "psql", "-c", createUser); err != nil {
		return fmt.Errorf("create postgres user: %w", err)
	}
	if err := system.SudoRun("-u", "postgres", "psql", "-c",
		"CREATE DATABASE lndhub OWNER lndhub"); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	logger.Install("PostgreSQL database and user created")
	return nil
}

// ── Git ─────────────────────────────────────────────────

func installGit() error {
	return system.SudoRun("apt-get", "install", "-y", "-qq", "git")
}

// ── Build from source ────────────────────────────────────

// lndhubBuildDir holds the temp directory used across clone/build/install.
var lndhubBuildDir string

func cloneLndHub() error {
	buildDir, err := os.MkdirTemp("", "rlvpn-lndhub-")
	if err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}
	lndhubBuildDir = buildDir

	repoDir := filepath.Join(buildDir, "lndhub.go")
	if err := system.Run("git", "clone", "--branch", lndhubVersion,
		"--depth", "1", lndhubRepo, repoDir); err != nil {
		os.RemoveAll(buildDir)
		lndhubBuildDir = ""
		return fmt.Errorf("clone lndhub.go: %w", err)
	}
	logger.Install("Cloned lndhub.go at tag %s", lndhubVersion)
	return nil
}

func buildLndHub() error {
	if lndhubBuildDir == "" {
		return fmt.Errorf("build dir not set — run cloneLndHub first")
	}
	goPath := goInstallDir + "/bin/go"
	repoDir := filepath.Join(lndhubBuildDir, "lndhub.go")
	goCacheDir := filepath.Join(lndhubBuildDir, "go-cache")
	goPathDir := filepath.Join(lndhubBuildDir, "go-path")

	// buildLndHub uses exec.Command directly because it needs
	// custom Dir and Env fields that system.Run does not support.
	cmd := exec.Command(goPath, "build", "-trimpath",
		"-ldflags=-s -w",
		"-o", filepath.Join(repoDir, "lndhub"),
		"./cmd/server/")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"GOPATH="+goPathDir,
		"GOCACHE="+goCacheDir,
		"PATH="+goInstallDir+"/bin:"+os.Getenv("PATH"),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build lndhub: %s: %s", err, output)
	}

	logger.Install("Built lndhub from source")
	return nil
}

func installLndHubBinary() error {
	if lndhubBuildDir == "" {
		return fmt.Errorf("build dir not set — run cloneLndHub first")
	}
	binaryPath := filepath.Join(lndhubBuildDir, "lndhub.go", "lndhub")
	if err := system.SudoRun("install", "-m", "0755", "-o", "root", "-g", "root",
		binaryPath, "/usr/local/bin/lndhub"); err != nil {
		return err
	}

	os.RemoveAll(lndhubBuildDir)
	lndhubBuildDir = ""

	logger.Install("Installed lndhub binary")
	return nil
}

// ── Macaroon ─────────────────────────────────────────────

func bakeLndHubMacaroon(cfg *config.AppConfig) error {
	net := cfg.NetworkConfig()

	output, err := system.SudoRunCombinedOutput("-u", systemUser, "lncli",
		"--lnddir="+paths.LNDDataDir,
		"--network="+net.LNCLINetwork,
		"bakemacaroon",
		"--save_to="+paths.LndHubMacaroon,
		"info:read", "invoices:read", "invoices:write",
		"offchain:read", "offchain:write")
	if err != nil {
		return fmt.Errorf("bake macaroon: %s: %s", err, output)
	}

	if err := system.SudoRun("chmod", "0640", paths.LndHubMacaroon); err != nil {
		return err
	}
	if err := system.SudoRun("chown", systemUser+":"+systemUser, paths.LndHubMacaroon); err != nil {
		return err
	}

	logger.Install("Baked LndHub macaroon with restricted permissions")
	return nil
}

// ── Directories ──────────────────────────────────────────

func createLndHubDirs() error {
	dirs := []struct {
		path  string
		owner string
		mode  os.FileMode
	}{
		{paths.LndHubDir, "root:" + systemUser, 0750},
		{paths.LndHubDataDir, systemUser + ":" + systemUser, 0750},
	}
	for _, d := range dirs {
		if err := system.SudoRun("mkdir", "-p", d.path); err != nil {
			return err
		}
		if err := system.SudoRun("chown", d.owner, d.path); err != nil {
			return err
		}
		if err := system.SudoRun("chmod", fmt.Sprintf("%o", d.mode), d.path); err != nil {
			return err
		}
	}
	return nil
}

// ── Configuration ────────────────────────────────────────

func writeLndHubConfig(cfg *config.AppConfig, dbPassword, jwtSecret, adminToken string) error {
	content := fmt.Sprintf(`# Virtual Private Node — LndHub.go
DATABASE_URI=postgresql://lndhub:%s@localhost:5432/lndhub?sslmode=disable
JWT_SECRET=%s
JWT_ACCESS_EXPIRY=172800
JWT_REFRESH_EXPIRY=604800
LND_ADDRESS=localhost:10009
LND_MACAROON_FILE=%s
LND_CERT_FILE=%s
HOST=127.0.0.1
PORT=%s
ENABLE_PROMETHEUS=false
ALLOW_ACCOUNT_CREATION=true
ADMIN_TOKEN=%s
FEE_RESERVE=false
`, dbPassword, jwtSecret, paths.LndHubMacaroon, paths.LNDTLSCert,
		paths.LndHubInternalPort, adminToken)

	if err := system.SudoWriteFile(paths.LndHubEnv, []byte(content), 0640); err != nil {
		return err
	}
	return system.SudoRun("chown", "root:"+systemUser, paths.LndHubEnv)
}

// ── Systemd ──────────────────────────────────────────────

func writeLndHubService() error {
	content := fmt.Sprintf(`[Unit]
Description=LndHub.go Lightning Accounts
After=lnd.service postgresql.service
Wants=lnd.service postgresql.service

[Service]
Type=simple
User=%s
Group=%s
EnvironmentFile=%s
ExecStart=/usr/local/bin/lndhub
Restart=on-failure
RestartSec=30
TimeoutStopSec=120
PrivateTmp=true
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, systemUser, systemUser, paths.LndHubEnv)
	return system.SudoWriteFile(paths.LndHubService, []byte(content), 0644)
}

func startLndHub() error {
	if err := system.SudoRun("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := system.SudoRun("systemctl", "enable", "lndhub"); err != nil {
		return err
	}
	return system.SudoRun("systemctl", "start", "lndhub")
}

// ── Account creation ─────────────────────────────────────

type LndHubAccount struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func CreateLndHubAccount(adminToken string) (*LndHubAccount, error) {
	output, err := system.RunContext(10*time.Second, "curl", "-s",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", "Authorization: Bearer "+adminToken,
		"-d", "{}",
		"http://127.0.0.1:"+paths.LndHubInternalPort+"/create")
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	var account LndHubAccount
	if err := json.Unmarshal([]byte(output), &account); err != nil {
		return nil, fmt.Errorf("parse response: %w (%s)", err, output)
	}

	if account.Login == "" {
		return nil, fmt.Errorf("empty login in response: %s", output)
	}

	return &account, nil
}

// ── Balance query ────────────────────────────────────────

// GetUserBalance queries the LndHub PostgreSQL database for a user's balance.
// Uses psql variable binding to prevent SQL injection.
func GetUserBalance(login string) (string, error) {
	if err := validateLogin(login); err != nil {
		return "unknown", fmt.Errorf("balance query: %w", err)
	}

	// validateLogin guarantees login is [a-zA-Z0-9]{1,40},
	// making SQL injection impossible through this value.
	query := fmt.Sprintf(`SELECT COALESCE(
        (SELECT SUM(te.amount) FROM transaction_entries te WHERE te.credit_account_id = a.id) -
        (SELECT SUM(te.amount) FROM transaction_entries te WHERE te.debit_account_id = a.id), 0)
        FROM accounts a JOIN users u ON a.user_id = u.id
        WHERE u.login = '%s' AND a.type = 'current'`, login)

	output, err := system.RunContext(10*time.Second,
		"sudo", "-u", "postgres", "psql",
		"-t", "-A", "lndhub",
		"-c", query)
	if err != nil {
		return "unknown", nil
	}

	balance := strings.TrimSpace(output)
	if balance == "" {
		return "0", nil
	}
	return balance, nil
}

// ── Deactivation ─────────────────────────────────────────

// DeactivateUser sets the deactivated flag on a user in the LndHub database.
// Uses psql variable binding to prevent SQL injection.
func DeactivateUser(login string) error {
	if err := validateLogin(login); err != nil {
		return fmt.Errorf("deactivate: %w", err)
	}

	// validateLogin guarantees login is [a-zA-Z0-9]{1,40},
	// making SQL injection impossible through this value.
	_, err := system.RunContext(10*time.Second,
		"sudo", "-u", "postgres", "psql",
		"lndhub",
		"-c", fmt.Sprintf("UPDATE users SET deactivated = true WHERE login = '%s'", login))
	if err != nil {
		return fmt.Errorf("deactivate user: %w", err)
	}

	logger.TUI("Deactivated LndHub user: %s", login)
	return nil
}
