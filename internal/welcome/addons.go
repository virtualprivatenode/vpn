// internal/welcome/addons.go

package welcome

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) viewAddons(bw int) string {
	halfW := (bw - 2) / 2
	cardH := theme.BoxHeight

	syncCard := m.addonSyncthingCard(halfW, cardH)
	hubCard := m.addonLndHubCard(halfW, cardH)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		syncCard, "  ", hubCard)
}

func (m Model) addonSyncthingCard(w, h int) string {
	var lines []string
	lines = append(lines, theme.Header.Render("🔄 Syncthing"))
	lines = append(lines, "")
	lines = append(lines, theme.Dim.Render("Auto-backup LND"))
	lines = append(lines, theme.Dim.Render("channel state to"))
	lines = append(lines, theme.Dim.Render("your local device."))
	lines = append(lines, "")

	if m.cfg.SyncthingInstalled {
		lines = append(lines, theme.GreenDot.Render("●")+" "+
			theme.Good.Render("Installed"))
		lines = append(lines, "")
		lines = append(lines, theme.Label.Render("Version:"))
		lines = append(lines, theme.Value.Render(getSyncthingVersion()))
		lines = append(lines, "")
		lines = append(lines, theme.Label.Render("Connections:"))
		lines = append(lines, theme.Value.Render(
			fmt.Sprintf("%d paired", len(m.cfg.SyncthingDevices))))
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render("Select for details ▸"))
	} else if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Grayed.Render("Requires: "))
		lines = append(lines, theme.Grayed.Render("LND + Wallet"))
	} else {
		lines = append(lines, theme.RedDot.Render("●")+" "+
			theme.Dim.Render("Not installed"))
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render("Select to install ▸"))
	}

	border := theme.NormalBorder
	if m.addonFocus == 0 {
		if (m.cfg.HasLND() && m.cfg.WalletExists()) ||
			m.cfg.SyncthingInstalled {
			border = theme.SelectedBorder
		} else {
			border = theme.GrayedBorder
		}
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) addonLndHubCard(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡️ LndHub"))
	lines = append(lines, "")
	lines = append(lines, theme.Dim.Render("Lightning accounts"))
	lines = append(lines, theme.Dim.Render("for family and"))
	lines = append(lines, theme.Dim.Render("friends."))
	lines = append(lines, "")

	if m.cfg.LndHubInstalled {
		activeCount := 0
		for _, a := range m.cfg.LndHubAccounts {
			if a.Active {
				activeCount++
			}
		}
		lines = append(lines, theme.GreenDot.Render("●")+" "+
			theme.Good.Render("Installed"))
		lines = append(lines, "")
		lines = append(lines, theme.Label.Render("Version:"))
		lines = append(lines,
			theme.Value.Render("v"+installer.LndHubVersionStr()))
		lines = append(lines, "")
		lines = append(lines, theme.Label.Render("Accounts:"))
		lines = append(lines,
			theme.Value.Render(fmt.Sprintf("%d active", activeCount)))
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render("Select to manage ▸"))
	} else if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		lines = append(lines, theme.Grayed.Render("Requires: "))
		lines = append(lines, theme.Grayed.Render("LND + Wallet"))
	} else if m.status != nil && !m.status.btcSynced {
		lines = append(lines, theme.Grayed.Render("Waiting for Bitcoin"))
		lines = append(lines, theme.Grayed.Render("to finish syncing"))
	} else {
		lines = append(lines, theme.RedDot.Render("●")+" "+
			theme.Dim.Render("Not installed"))
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render("Select to install ▸"))
	}

	border := theme.NormalBorder
	if m.addonFocus == 1 {
		if m.cfg.LndHubInstalled ||
			(m.cfg.HasLND() && m.cfg.WalletExists() &&
				(m.status == nil || m.status.btcSynced)) {
			border = theme.SelectedBorder
		} else {
			border = theme.GrayedBorder
		}
	}
	return border.Width(w).Padding(1, 2).
		Render(padLines(lines, h))
}

func (m Model) viewSyncthingDetail() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines,
		theme.Header.Render("🔄 Syncthing — Channel Backup"))
	lines = append(lines, "")

	// Connection count
	pairedCount := m.syncthingPairedCount()
	lines = append(lines, theme.Header.Render(
		fmt.Sprintf("  Connections (%d paired)", pairedCount)))
	lines = append(lines, "  "+
		theme.Dim.Render("─────────────────────────────────"))

	if pairedCount == 0 {
		lines = append(lines, "  "+
			theme.Dim.Render("No devices paired yet"))
	} else {
		for i, d := range m.cfg.SyncthingDevices {
			prefix := "  "
			style := theme.Value
			if m.syncCursor == i {
				prefix = "▸ "
				style = theme.Action
			}
			lines = append(lines, fmt.Sprintf("  %s%s %s  %s",
				prefix,
				theme.GreenDot.Render("●"),
				style.Render(d.Name),
				theme.Dim.Render(d.PairedAt)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, theme.Header.Render("  Local Setup:"))
	lines = append(lines, "  "+theme.Dim.Render(
		"1. download & verify Syncthing — syncthing.net"))
	lines = append(lines, "  "+theme.Dim.Render(
		"2. ⚙ Actions → ⚙ Settings → Connections → unselect:"))
	lines = append(lines, "  "+theme.Dim.Render(
		"   ☐ Enable NAT traversal"))
	lines = append(lines, "  "+theme.Dim.Render(
		"   ☐ Global Discovery"))
	lines = append(lines, "  "+theme.Dim.Render(
		"   ☐ Local Discovery"))
	lines = append(lines, "  "+theme.Dim.Render(
		"   ☐ Enable Relaying"))
	lines = append(lines, "  "+theme.Dim.Render(
		"3. ✓ Save"))
	lines = append(lines, "  "+theme.Dim.Render(
		"4. ⚙ Actions → Show ID → Copy"))
	lines = append(lines, "  "+theme.Dim.Render(
		"5. Press [a] below to pair"))

	// Show VPS Device ID with QR for easy pairing
	vpsDeviceID := installer.GetSyncthingDeviceID()
	if vpsDeviceID != "" {
		lines = append(lines, "")
		lines = append(lines, theme.Header.Render("  This Node's Device ID:"))
		lines = append(lines, "  "+theme.Mono.Render(vpsDeviceID))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Dim.Render(
			"Scan QR with phone → paste into local Syncthing"))
		lines = append(lines, "  "+
			theme.Action.Render("[d] show Device ID QR"))
	}

	lines = append(lines, "")
	lines = append(lines, "  "+
		theme.Action.Render("[a] pair device"))
	lines = append(lines, "  "+
		theme.Action.Render("[u] web UI (Tor)"))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" 🔄 Syncthing Details ")
	footer := theme.Footer.Render(
		"  ↑↓ select • a pair • d QR • enter details • u web UI • backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewSyncthingPairInput() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines, theme.Header.Render("Pair Device"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Device ID: "))
	lines = append(lines, "  "+
		theme.Value.Render(m.syncDeviceInput+"_"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"Paste your local Syncthing Device ID."))
	lines = append(lines, "  "+theme.Dim.Render(
		"From local Syncthing: ⚙ Actions → Show ID"))

	if m.syncPairError != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+
			theme.Warning.Render("Error: "+m.syncPairError))
	}
	if m.syncPairSuccess {
		lines = append(lines, "")
		lines = append(lines, "  "+
			theme.Success.Render("✅ Device paired!"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Header.Render(
			"Now in your local Syncthing:"))
		lines = append(lines, "")

		vpsDeviceID := installer.GetSyncthingDeviceID()
		if vpsDeviceID != "" {
			lines = append(lines, "  "+theme.Dim.Render(
				"1. Add Remote Device"))
			lines = append(lines, "  "+theme.Label.Render(
				"   General → Device ID:"))
			lines = append(lines, "  "+theme.Mono.Render(
				"   "+vpsDeviceID))
			lines = append(lines, "  "+theme.Dim.Render(
				"2. Advanced → Addresses → replace dynamic with:"))
			lines = append(lines, "  "+theme.Mono.Render(
				"   tcp://<your-server-ip>:22000"))
			lines = append(lines, "  "+theme.Dim.Render(
				"3. Save → wait for connection"))
			lines = append(lines, "  "+theme.Dim.Render(
				"4. Accept the lnd-backup folder share"))
			lines = append(lines, "  "+theme.Dim.Render(
				"   General → set custom Folder Path"))
			lines = append(lines, "  "+theme.Dim.Render(
				"   Advanced → Folder Type → Receive Only"))
			lines = append(lines, "  "+theme.Dim.Render(
				"   ✓ Save"))
		}
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" 🔄 Pair Device ")
	var footer string
	if m.syncPairSuccess {
		footer = theme.Footer.Render(
			"  enter done • backspace back  ")
	} else {
		footer = theme.Footer.Render(
			"  enter pair • backspace cancel  ")
	}
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewSyncthingDeviceDetail() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	if m.syncCursor >= len(m.cfg.SyncthingDevices) {
		lines = append(lines,
			theme.Warn.Render("Device not found"))
	} else {
		dev := m.cfg.SyncthingDevices[m.syncCursor]
		lines = append(lines,
			theme.Header.Render("  "+dev.Name))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("Device ID:"))
		lines = append(lines, "  "+theme.Mono.Render(dev.DeviceID))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("Paired: ")+
			theme.Value.Render(dev.PairedAt))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" 🔄 Device Details ")
	footer := theme.Footer.Render(
		"  backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewSyncthingWebUI() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines,
		theme.Header.Render("🔄 Syncthing Web UI (Tor)"))
	lines = append(lines, "")

	syncOnion := readOnion(paths.TorSyncthingHostname)
	if syncOnion == "" {
		lines = append(lines, "  "+theme.Warn.Render(
			"Tor address not available yet."))
	} else {
		lines = append(lines, "  "+theme.Label.Render("URL:"))
		if m.showSecrets {
			lines = append(lines, "  "+theme.Mono.Render(
				"http://"+syncOnion+":8384"))
		}
		lines = append(lines, "  "+
			theme.Action.Render("[u] full URL for copy/paste"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("User: ")+
			theme.Mono.Render("admin"))
		if m.cfg.SyncthingPassword != "" {
			if m.showSecrets {
				lines = append(lines, "  "+theme.Label.Render("Pass: ")+
					theme.Mono.Render(m.cfg.SyncthingPassword))
				lines = append(lines, "  "+
					theme.Action.Render("[s] hide password"))
			} else {
				lines = append(lines, "  "+theme.Label.Render("Pass: ")+
					theme.Dim.Render("••••••••"))
				lines = append(lines, "  "+
					theme.Action.Render("[s] show password"))
			}
		}
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Dim.Render(
			"Open in Tor Browser for advanced settings."))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" 🔄 Syncthing Web UI ")
	footer := theme.Footer.Render(
		"  u full URL • s toggle password • backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) syncthingPairedCount() int {
	return len(m.cfg.SyncthingDevices)
}

// ── LndHub management screens ────────────────────────────

func (m Model) viewLndHubManage() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines, theme.Lightning.Render(
		"⚡️ LndHub — Account Management"))
	lines = append(lines, "")

	accounts := m.cfg.LndHubAccounts
	activeCount := 0
	for _, a := range accounts {
		if a.Active {
			activeCount++
		}
	}
	lines = append(lines, theme.Header.Render(
		fmt.Sprintf("  Accounts (%d active)", activeCount)))
	lines = append(lines, "  "+
		theme.Dim.Render("─────────────────────────────────"))

	if len(accounts) == 0 {
		lines = append(lines, "  "+
			theme.Dim.Render("No accounts yet"))
	} else {
		viewStart := 0
		viewSize := 8
		if m.hubCursor >= viewStart+viewSize {
			viewStart = m.hubCursor - viewSize + 1
		}
		if viewStart < 0 {
			viewStart = 0
		}
		viewEnd := viewStart + viewSize
		if viewEnd > len(accounts) {
			viewEnd = len(accounts)
		}

		if viewStart > 0 {
			lines = append(lines, "  "+
				theme.Dim.Render("  ↑ more"))
		}

		for i := viewStart; i < viewEnd; i++ {
			a := accounts[i]
			prefix := "  "
			style := theme.Value
			if m.hubCursor == i {
				prefix = "▸ "
				style = theme.Action
			}
			status := theme.GreenDot.Render("●")
			if !a.Active {
				status = theme.RedDot.Render("●")
			}
			lines = append(lines, fmt.Sprintf("  %s%s %s  %s",
				prefix, status, style.Render(a.Label),
				theme.Dim.Render(a.CreatedAt)))
		}

		if viewEnd < len(accounts) {
			lines = append(lines, "  "+
				theme.Dim.Render("  ↓ more"))
		}
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡️ LndHub Management ")
	footer := theme.Footer.Render(
		"  ↑↓ select • c create • enter details • x deactivate • backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewLndHubCreateName() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines, theme.Header.Render("Create Account"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Label.Render("Name: ")+
		theme.Value.Render(m.hubNameInput+"_"))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.Dim.Render(
		"Letters, numbers, spaces, hyphens"))
	lines = append(lines, "  "+theme.Dim.Render(
		"Press enter to create"))

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡️ New Account ")
	footer := theme.Footer.Render(
		"  enter create • backspace cancel  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewLndHubNewAccount() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines, theme.Success.Render(
		"✅ Account created: "+m.hubNameInput))
	lines = append(lines, "")

	if m.lastAccount != nil {
		hubOnion := readOnion(paths.TorLndHubHostname)
		if hubOnion != "" {
			lines = append(lines, "  "+theme.Label.Render("Tor:"))
			lines = append(lines, "  "+theme.Mono.Render(
				hubOnion+":"+paths.LndHubExternalPort))
		}
		if m.cfg.P2PMode == "hybrid" && m.status != nil &&
			m.status.publicIP != "" {
			lines = append(lines,
				"  "+theme.Label.Render("Clearnet (HTTPS):"))
			lines = append(lines, "  "+theme.Mono.Render(
				m.status.publicIP+":"+paths.LndHubExternalPort))
		}
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("Login:    ")+
			theme.Mono.Render(m.lastAccount.Login))
		lines = append(lines, "  "+theme.Label.Render("Password: ")+
			theme.Mono.Render(m.lastAccount.Password))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Warning.Render(
			"Share these credentials with "+m.hubNameInput+"."))
		lines = append(lines, "  "+theme.Warning.Render(
			"They will not be shown again."))
		lines = append(lines, "")

		if hubOnion != "" {
			lines = append(lines, "  "+
				theme.Action.Render("[r] QR code (Tor)"))
		}
		if m.cfg.P2PMode == "hybrid" {
			lines = append(lines, "  "+
				theme.Action.Render("[c] QR code (Clearnet)"))
		}
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡️ Account Created ")
	var footer string
	if m.cfg.P2PMode == "hybrid" {
		footer = theme.Footer.Render(
			"  r QR (Tor) • c QR (Clearnet) • enter done  ")
	} else {
		footer = theme.Footer.Render(
			"  r QR (Tor) • enter done  ")
	}
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewLndHubAccountDetail() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	if m.hubCursor >= len(m.cfg.LndHubAccounts) {
		lines = append(lines,
			theme.Warn.Render("Account not found"))
	} else {
		acct := m.cfg.LndHubAccounts[m.hubCursor]
		lines = append(lines,
			theme.Header.Render("  "+acct.Label))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Label.Render("Login:   ")+
			theme.Mono.Render(acct.Login))
		lines = append(lines, "  "+theme.Label.Render("Created: ")+
			theme.Value.Render(acct.CreatedAt))

		if acct.Active {
			lines = append(lines, "  "+theme.Label.Render("Status:  ")+
				theme.Success.Render("active"))
		} else {
			lines = append(lines, "  "+theme.Label.Render("Status:  ")+
				theme.Warning.Render("deactivated"))
			if acct.DeactivatedAt != "" {
				lines = append(lines,
					"  "+theme.Label.Render("Deactivated: ")+
						theme.Value.Render(acct.DeactivatedAt))
			}
			if acct.BalanceOnDeactivate != "" &&
				acct.BalanceOnDeactivate != "0" &&
				acct.BalanceOnDeactivate != "unknown" {
				lines = append(lines, "")
				lines = append(lines, "  "+theme.Warning.Render(
					"Had "+acct.BalanceOnDeactivate+
						" sats at deactivation."))
				lines = append(lines, "  "+theme.Warning.Render(
					"Send this amount to their new account."))
			}
		}
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡️ Account Details ")
	footer := theme.Footer.Render(
		"  backspace back • q quit  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewLndHubDeactivateConfirm() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string

	if m.hubCursor < len(m.cfg.LndHubAccounts) {
		acct := m.cfg.LndHubAccounts[m.hubCursor]
		lines = append(lines, theme.Warning.Render(
			"Deactivate "+acct.Label+"?"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Value.Render("This will:"))
		lines = append(lines, "  "+theme.Value.Render(
			"• Immediately block their wallet access"))
		lines = append(lines, "  "+theme.Value.Render(
			"• Record their balance at deactivation"))
		lines = append(lines, "  "+theme.Value.Render(
			"• Their login credentials stop working"))
		lines = append(lines, "")
		lines = append(lines, "  "+theme.Dim.Render(
			"Create them a new account afterward"))
		lines = append(lines, "  "+theme.Dim.Render(
			"and send their old balance to it."))
	}

	box := theme.Box.Width(bw).Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).
		Render(" ⚡️ Deactivate Account ")
	footer := theme.Footer.Render(
		"  y confirm • n cancel  ")
	full := lipgloss.JoinVertical(lipgloss.Center,
		"", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewSyncthingDeviceQR() string {
	vpsDeviceID := installer.GetSyncthingDeviceID()

	var lines []string
	lines = append(lines, theme.Header.Render(
		"🔄 Node Device ID — Scan to Pair"))
	lines = append(lines, "")

	if vpsDeviceID == "" {
		lines = append(lines, theme.Warn.Render(
			"Device ID not available."))
	} else {
		qr := renderQRCode(vpsDeviceID)
		if qr != "" {
			lines = append(lines, qr)
		}
		lines = append(lines, "")
		lines = append(lines, theme.Dim.Render(
			"Scan → copy Device ID → paste into local Syncthing"))
		lines = append(lines, theme.Dim.Render(
			"Add Remote Device → Device ID field"))
	}

	lines = append(lines, "")
	lines = append(lines, theme.Footer.Render(
		"backspace back • q quit"))
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Left, lines...))
}
