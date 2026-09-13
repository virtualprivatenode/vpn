// internal/system/info.go

package system

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type DiskInfo struct {
	Total   string
	Used    string
	Percent string
}

type MemInfo struct {
	Total   string
	Used    string
	Percent string
}

func ReadDisk(ctx context.Context, path string) (DiskInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "df", "-h", "--output=size,used,pcent", path).Output()
	if err != nil {
		return DiskInfo{}, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return DiskInfo{}, errors.New("missing disk usage")
	}
	f := strings.Fields(lines[1])
	if len(f) != 3 {
		return DiskInfo{}, errors.New("invalid disk usage")
	}
	return DiskInfo{f[0], f[1], f[2]}, nil
}

func ReadMemory() (MemInfo, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemInfo{}, err
	}
	return parseMemory(string(data))
}

func parseMemory(data string) (MemInfo, error) {
	var total, avail int
	haveTotal, haveAvail := false, false
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			n, err := fmt.Sscanf(line, "MemTotal: %d kB", &total)
			haveTotal = n == 1 && err == nil
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			n, err := fmt.Sscanf(line, "MemAvailable: %d kB", &avail)
			haveAvail = n == 1 && err == nil
		}
	}
	if !haveTotal || !haveAvail || total <= 0 || avail < 0 || avail > total {
		return MemInfo{}, errors.New("invalid memory usage")
	}
	used := total - avail
	return MemInfo{Total: fmtKB(total), Used: fmtKB(used),
		Percent: fmt.Sprintf("%.0f%%", float64(used)/float64(total)*100)}, nil
}

// ReadServiceActive separates an inactive unit from a failed systemd query.
func ReadServiceActive(ctx context.Context, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "show", "--property=ActiveState", "--value", name).Output()
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(string(out)) {
	case "active", "reloading", "refreshing":
		return true, nil
	case "inactive", "failed", "activating", "deactivating", "maintenance":
		return false, nil
	default:
		return false, errors.New("unavailable service state")
	}
}

func ReadRebootRequired() (bool, error) {
	_, err := os.Stat("/var/run/reboot-required")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// DirSize measures a directory tree with du. Root only: the
// data directories it is used on belong to service users. The
// unprivileged status screen gets the LND size through the
// helper's dir-size operation (which calls this as root) and
// the Bitcoin size from bitcoind's own RPC — it never calls
// this directly.
func DirSize(path string) string {
	if os.Geteuid() != 0 {
		return "N/A"
	}
	out, err := exec.Command("du", "-sh", path).CombinedOutput()
	if err != nil {
		return "N/A"
	}
	f := strings.Fields(string(out))
	if len(f) < 1 {
		return "N/A"
	}
	return f[0]
}

func IsServiceActive(name string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
}

func ServiceAction(name, action string) error {
	return SudoRun("systemctl", action, name)
}

func RebootRequired() bool {
	_, err := os.Stat("/var/run/reboot-required")
	return err == nil
}

// ── Public IP detection ──────────────────────────────────

var (
	cachedIP string
	ipOnce   sync.Once
)

// PublicIPv4 returns the server's public IPv4 address.
// Uses the kernel routing table (no network call) and caches the result.
// Only relevant in hybrid (clearnet+tor) P2P mode.
func PublicIPv4() string {
	ipOnce.Do(func() {
		cachedIP = detectPublicIPv4()
	})
	return cachedIP
}

func detectPublicIPv4() string {
	ip, _ := ReadPublicIPv4(context.Background())
	return ip
}

func ReadPublicIPv4(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ip", "-4", "route", "get", "1.1.1.1").Output()
	if err != nil {
		return "", err
	}
	ip := ParseSourceIP(string(out))
	if ip == "" {
		return "", errors.New("public IPv4 unavailable")
	}
	return ip, nil
}

// ParseSourceIP extracts the source IP from "ip route get" output.
// Exported for testing.
func ParseSourceIP(routeOutput string) string {
	i := strings.Index(routeOutput, "src ")
	if i == -1 {
		return ""
	}
	fields := strings.Fields(routeOutput[i+4:])
	if len(fields) == 0 {
		return ""
	}
	ip := net.ParseIP(fields[0])
	if ip == nil || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return ""
	}
	return ip.String()
}

func fmtKB(kb int) string {
	if kb >= 1048576 {
		return fmt.Sprintf("%.1f GB", float64(kb)/1048576.0)
	}
	return fmt.Sprintf("%.0f MB", float64(kb)/1024.0)
}
