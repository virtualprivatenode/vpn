// internal/welcome/dashboard.go

package welcome

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/bitcoin"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) viewDashboard(bw int) string {
	halfW := (bw - 4) / 2
	cardH := theme.BoxHeight / 2

	svc := m.cardServicesView(halfW, cardH)
	sys := m.cardSystemView(halfW, cardH)
	btc := m.cardBitcoinView(halfW, cardH)
	ln := m.cardLightningView(halfW, cardH)

	top := lipgloss.JoinHorizontal(lipgloss.Top, svc, "  ", sys)
	bot := lipgloss.JoinHorizontal(lipgloss.Top, btc, "  ", ln)
	return lipgloss.JoinVertical(lipgloss.Left, top, "", bot)
}

func (m Model) getBorder(pos cardPos) lipgloss.Style {
	if m.activeTab == tabDashboard && m.dashCard == pos {
		return theme.SelectedBorder
	}
	return theme.NormalBorder
}

func (m Model) cardServicesView(w, h int) string {
	var lines []string
	lines = append(lines, theme.Header.Render(" Services"))
	lines = append(lines, "")

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
		if m.cardActive && m.dashCard == cardServices && m.svcCursor == i {
			prefix = "▸ "
			style = theme.Action
		}
		lines = append(lines, prefix+dot+" "+style.Render(name))
	}

	if m.cardActive && m.dashCard == cardServices {
		lines = append(lines, "")
		if m.svcConfirm != "" {
			svc := m.svcName(m.svcCursor)
			lines = append(lines, theme.Warning.Render(
				fmt.Sprintf("%s %s? [y/n]", m.svcConfirm, svc)))
		} else {
			lines = append(lines,
				theme.Dim.Render("[r]estart [s]top [a]start [l]ogs"))
		}
	}

	return m.getBorder(cardServices).Width(w).
		Padding(0, 1).Render(padLines(lines, h))
}

func (m Model) cardSystemView(w, h int) string {
	var lines []string
	lines = append(lines, theme.Header.Render(" System"))
	lines = append(lines, "")

	if m.status != nil {
		lines = append(lines, theme.Label.Render("Disk: ")+
			theme.Value.Render(fmt.Sprintf("%s / %s (%s)",
				m.status.diskUsed, m.status.diskTotal,
				m.status.diskPct)))
		lines = append(lines, theme.Label.Render("RAM:  ")+
			theme.Value.Render(fmt.Sprintf("%s / %s (%s)",
				m.status.ramUsed, m.status.ramTotal,
				m.status.ramPct)))
		lines = append(lines, theme.Label.Render("Bitcoin: ")+
			theme.Value.Render(m.status.btcSize))
		if m.cfg.HasLND() {
			lines = append(lines, theme.Label.Render("LND: ")+
				theme.Value.Render(m.status.lndSize))
		}
	} else {
		lines = append(lines, theme.Dim.Render("Loading..."))
	}

	if m.cardActive && m.dashCard == cardSystem {
		lines = append(lines, "")
		if m.sysConfirm != "" {
			lines = append(lines, theme.Warning.Render(
				fmt.Sprintf("%s system? [y/n]", m.sysConfirm)))
		} else {
			lines = append(lines,
				theme.Action.Render("[u]pdate packages"))
			if m.status != nil && m.status.rebootRequired {
				lines = append(lines,
					theme.Warning.Render("⚠️ Reboot required"))
				lines = append(lines,
					theme.Action.Render("[r]eboot"))
			}
		}
	} else if m.status != nil && m.status.rebootRequired {
		lines = append(lines, "")
		lines = append(lines,
			theme.Warning.Render("⚠️ Reboot required"))
	}

	return m.getBorder(cardSystem).Width(w).
		Padding(0, 1).Render(padLines(lines, h))
}

func (m Model) cardBitcoinView(w, h int) string {
	var lines []string
	lines = append(lines, theme.Bitcoin.Render("₿ Bitcoin"))
	lines = append(lines, "")

	if m.status == nil {
		lines = append(lines, theme.Dim.Render("Loading..."))
	} else if !m.status.btcResponding {
		lines = append(lines, theme.Warn.Render("Not responding"))
	} else {
		if m.status.btcSynced {
			lines = append(lines, theme.Label.Render("Sync: ")+
				theme.Good.Render("✅ synced"))
		} else {
			lines = append(lines, theme.Label.Render("Sync: ")+
				theme.Warn.Render("🔄 syncing"))
		}
		lines = append(lines, theme.Label.Render("Height: ")+
			theme.Value.Render(fmt.Sprintf("%d / %d",
				m.status.btcBlocks, m.status.btcHeaders)))
		if m.status.btcProgress > 0 {
			lines = append(lines, theme.Label.Render("Progress: ")+
				theme.Value.Render(
					bitcoin.FormatProgress(m.status.btcProgress)))
		}
		lines = append(lines, theme.Label.Render("Network: ")+
			theme.Value.Render(m.cfg.Network))
	}

	return m.getBorder(cardBitcoin).Width(w).
		Padding(0, 1).Render(padLines(lines, h))
}

func (m Model) cardLightningView(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡️ Lightning"))
	lines = append(lines, "")

	if !m.cfg.HasLND() {
		lines = append(lines, theme.Grayed.Render("LND not installed"))
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render("Select to install ▸"))
	} else if !m.cfg.WalletExists() {
		lines = append(lines, theme.Label.Render("Wallet: ")+
			theme.Warning.Render("not created"))
		lines = append(lines, theme.Label.Render("P2P: ")+
			theme.Value.Render(p2pModeLabel(m.cfg.P2PMode)))
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render("Select to create ▸"))
	} else {
		lines = append(lines, theme.Label.Render("Wallet: ")+
			theme.Success.Render("created"))
		if m.cfg.AutoUnlock {
			lines = append(lines, theme.Label.Render("Auto-unlock: ")+
				theme.Success.Render("enabled"))
		}
		lines = append(lines, theme.Label.Render("P2P: ")+
			theme.Value.Render(p2pModeLabel(m.cfg.P2PMode)))
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render(
			"Select for Lightning tab ▸"))
	}

	return m.getBorder(cardLightning).Width(w).
		Padding(0, 1).Render(padLines(lines, h))
}

func p2pModeLabel(mode string) string {
	if mode == "hybrid" {
		return "Tor + clearnet"
	}
	return "Tor only"
}
