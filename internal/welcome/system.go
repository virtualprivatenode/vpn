package welcome

import (
	"fmt"
	"strings"

	"github.com/ripsline/virtual-private-node/internal/bitcoin"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

// systemOverview shows all system info on one page + update button
func (m Model) systemOverview(w, h int) string {
	var lines []string

	// Services
	lines = append(lines, theme.Header.Render(" Services"))
	names := serviceNames(m.cfg)
	for i, name := range names {
		dot := theme.RedDot.Render("●")
		if m.status != nil {
			if active, ok := m.status.services[name]; ok && active {
				dot = theme.GreenDot.Render("●")
			}
		}
		prefix := "  "
		style := theme.Value
		if m.svcCursor == i && m.contentFocused {
			prefix = "▸ "
			style = theme.Action
		}
		lines = append(lines, prefix+dot+" "+style.Render(name))
	}

	if m.svcConfirm != "" {
		svc := m.svcName(m.svcCursor)
		lines = append(lines, "")
		lines = append(lines, " "+theme.Warning.Render(
			fmt.Sprintf("%s %s? [y/n]", m.svcConfirm, svc)))
	}

	lines = append(lines, "")

	// Stats
	lines = append(lines, theme.Header.Render(" System"))
	if m.status != nil {
		lines = append(lines,
			" "+theme.Label.Render("Disk: ")+
				theme.Value.Render(fmt.Sprintf("%s / %s (%s)",
					m.status.diskUsed, m.status.diskTotal,
					m.status.diskPct)))
		lines = append(lines,
			" "+theme.Label.Render("RAM:  ")+
				theme.Value.Render(fmt.Sprintf("%s / %s (%s)",
					m.status.ramUsed, m.status.ramTotal,
					m.status.ramPct)))
		lines = append(lines,
			" "+theme.Label.Render("BTC:  ")+
				theme.Value.Render(m.status.btcSize))
		if m.cfg.HasLND() {
			lines = append(lines,
				" "+theme.Label.Render("LND:  ")+
					theme.Value.Render(m.status.lndSize))
		}
	} else {
		lines = append(lines, " "+theme.Dim.Render("Loading..."))
	}

	if m.status != nil && m.status.rebootRequired {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Warning.Render("Reboot required"))
	}

	if m.sysConfirm != "" {
		lines = append(lines, "")
		lines = append(lines, " "+theme.Warning.Render(
			fmt.Sprintf("%s? [y/n]", m.sysConfirm)))
	}

	lines = append(lines, "")

	// Bitcoin
	lines = append(lines, theme.Bitcoin.Render(" ₿ Bitcoin"))
	if m.status == nil {
		lines = append(lines, " "+theme.Dim.Render("Loading..."))
	} else if !m.status.btcResponding {
		lines = append(lines, " "+theme.Warn.Render("Not responding"))
	} else {
		if m.status.btcSynced {
			lines = append(lines,
				" "+theme.Label.Render("Sync: ")+
					theme.Good.Render("synced"))
		} else {
			lines = append(lines,
				" "+theme.Label.Render("Sync: ")+
					theme.Warn.Render("syncing"))
		}
		lines = append(lines,
			" "+theme.Label.Render("Height: ")+
				theme.Value.Render(fmt.Sprintf("%d / %d",
					m.status.btcBlocks, m.status.btcHeaders)))
		if m.status.btcProgress > 0 {
			lines = append(lines,
				" "+theme.Label.Render("Progress: ")+
					theme.Value.Render(
						bitcoin.FormatProgress(m.status.btcProgress)))
		}
		lines = append(lines,
			" "+theme.Label.Render("Network: ")+
				theme.Value.Render(m.cfg.Network))
	}

	lines = append(lines, "")

	// Software + buttons
	lines = append(lines,
		" "+theme.Label.Render("VPN v")+
			theme.Value.Render(installer.GetVersion()))

	if m.latestVersion != "" &&
		m.latestVersion != installer.GetVersion() {
		lines = append(lines,
			" "+theme.Label.Render("Latest: ")+
				theme.Action.Render("v"+m.latestVersion))
	}

	if m.updateConfirm {
		lines = append(lines, "")
		lines = append(lines, " "+theme.Warning.Render(
			"Update to v"+m.latestVersion+"? [y/n]"))
	}

	lines = append(lines, "")
	lines = append(lines, m.systemButtons())

	return strings.Join(lines, "\n")
}

func (m Model) systemButtons() string {
	btnUpdate := " Update Software "
	btnPkgs := " Update Packages "
	btnReboot := " Reboot "

	maxBtn := 1
	if m.status != nil && m.status.rebootRequired {
		maxBtn = 2
	}

	switch m.btnIdx {
	case 0:
		btnUpdate = theme.ActiveTab.Render(btnUpdate)
		btnPkgs = theme.InactiveTab.Render(btnPkgs)
		btnReboot = theme.InactiveTab.Render(btnReboot)
	case 1:
		btnUpdate = theme.InactiveTab.Render(btnUpdate)
		btnPkgs = theme.ActiveTab.Render(btnPkgs)
		btnReboot = theme.InactiveTab.Render(btnReboot)
	case 2:
		btnUpdate = theme.InactiveTab.Render(btnUpdate)
		btnPkgs = theme.InactiveTab.Render(btnPkgs)
		btnReboot = theme.ActiveTab.Render(btnReboot)
	}

	_ = maxBtn
	result := " " + btnUpdate + "  " + btnPkgs
	if m.status != nil && m.status.rebootRequired {
		result += "  " + btnReboot
	}
	return result
}
