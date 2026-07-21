// internal/system/info.go

package system

import (
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

func Disk(path string) DiskInfo {
	cmd := exec.Command("df", "-h", "--output=size,used,pcent", path)
	out, _ := cmd.CombinedOutput()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return DiskInfo{"N/A", "N/A", "N/A"}
	}
	f := strings.Fields(lines[1])
	if len(f) < 3 {
		return DiskInfo{"N/A", "N/A", "N/A"}
	}
	return DiskInfo{f[0], f[1], f[2]}
}

func Memory() MemInfo {
	data, _ := os.ReadFile("/proc/meminfo")
	var total, avail int
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &total)
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &avail)
		}
	}
	if total == 0 {
		return MemInfo{"N/A", "N/A", "N/A"}
	}
	used := total - avail
	return MemInfo{
		Total:   fmtKB(total),
		Used:    fmtKB(used),
		Percent: fmt.Sprintf("%.0f%%", float64(used)/float64(total)*100),
	}
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
	output, err := RunContext(3*time.Second,
		"ip", "-4", "route", "get", "1.1.1.1")
	if err != nil {
		return ""
	}
	return ParseSourceIP(output)
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
