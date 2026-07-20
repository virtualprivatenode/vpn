// internal/installer/hardware.go

package installer

// The hardware-fit install step (ruling viii): detect what the
// box actually has — RAM from /proc/meminfo, disk from statfs on
// the data mount, cores — recommend bitcoind settings from it,
// and let the operator confirm. FIRST-BOOT class (ruling iv):
// images deploy onto unknown hardware, so this must never run at
// bake time.
//
// Scope now: dbcache sizing off RAM. Prune-vs-full is a separate
// pending product ruling (LND-on-pruned caveats — owned by the
// image-track/wizard session); post-IBD dbcache reduction is a
// later refinement. Post-commit-7 these settings become bounded
// typed params on the bitcoin config verb (principle 4).

import (
	"os"
	"runtime"
	"strings"
	"syscall"
)

// Hardware is the detected machine profile shown to the operator.
type Hardware struct {
	RAMMB       int // MemTotal
	DiskTotalGB int // filesystem holding the data dir
	DiskFreeGB  int
	Cores       int
}

// Product requirements (README): 2 (v)CPU, 4+ GB RAM, 90+ GB SSD.
// Detected values are compared against these for the fit display —
// shortfalls WARN, they do not refuse (the operator may know
// better; a testnet box needs far less disk).
const (
	requiredCores  = 2
	requiredRAMMB  = 3600 // "4 GB" boxes report ~3.8 GiB usable
	requiredDiskGB = 90
)

// DetectHardware reads the machine profile. Detection failures
// yield zero fields — rendered as "unknown", never fatal: the
// step informs a confirmation, and RecommendDbCache treats
// unknown RAM conservatively.
func DetectHardware() Hardware {
	hw := Hardware{Cores: runtime.NumCPU()}

	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		hw.RAMMB = parseMemTotalMB(string(data))
	}

	// The bitcoin data dir may not exist yet (fresh box) —
	// statfs the nearest existing ancestor. /var/lib exists on
	// any Debian system.
	for _, p := range []string{"/var/lib/bitcoin", "/var/lib", "/"} {
		var st syscall.Statfs_t
		if err := syscall.Statfs(p, &st); err == nil {
			hw.DiskTotalGB = int(uint64(st.Blocks) *
				uint64(st.Bsize) / 1_000_000_000)
			hw.DiskFreeGB = int(uint64(st.Bavail) *
				uint64(st.Bsize) / 1_000_000_000)
			break
		}
	}
	return hw
}

// parseMemTotalMB extracts MemTotal from /proc/meminfo content,
// in MB. Pure — unit-tested. Returns 0 when absent/garbled.
func parseMemTotalMB(meminfo string) int {
	for _, line := range strings.Split(meminfo, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb := 0
		for _, c := range fields[1] {
			if c < '0' || c > '9' {
				return 0
			}
			kb = kb*10 + int(c-'0')
		}
		return kb / 1024
	}
	return 0
}

// RecommendDbCache maps detected RAM to a bitcoind dbcache (MB).
// Pure — unit-tested. Conservative table, capped at 2048: IBD
// gains flatten beyond that on a node that also runs LND and Tor,
// and the box must keep headroom for lnd's own footprint.
//
//	unknown / < 3 GB → 512 (the historical hardcode)
//	≥ 3 GB           → 1024
//	≥ 6 GB           → 2048
func RecommendDbCache(ramMB int) int {
	switch {
	case ramMB >= 6144:
		return 2048
	case ramMB >= 3072:
		return 1024
	default:
		return 512
	}
}

// dbCacheChoices are the values the hardware screen cycles
// through — the recommendation is one of them.
var dbCacheChoices = []int{512, 1024, 2048}
