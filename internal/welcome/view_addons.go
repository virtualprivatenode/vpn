package welcome

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ripsline/virtual-private-node/internal/installer"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) renderSyncthingTabContent(
	w, h int,
) string {
	switch m.subview {
	case svSyncthingPairInput:
		return m.syncthingPairContent(w)
	case svSyncthingWebUI:
		return m.syncthingWebUIContent(w)
	case svSyncthingDeviceDetail:
		return m.syncthingDeviceDetailContent(w)
	case svSyncthingDeviceQR:
		return m.syncthingDetailContent(w, h)
	default:
		return m.syncthingDetailContent(w, h)
	}
}

func (m Model) renderLndHubTabContent(
	w, h int,
) string {
	switch m.subview {
	case svLndHubCreateName:
		return m.lndhubCreateNameContent(w)
	case svLndHubCreateAccount:
		return m.lndhubCreatedContent(w)
	case svLndHubAccountDetail:
		return m.lndhubAccountDetailContent(w)
	case svLndHubDeactivateConfirm:
		return m.lndhubDeactivateContent(w)
	default:
		return m.lndhubManageContent(w, h)
	}
}

func (m Model) addonsOverview(w, h int) string {
	var lines []string
	lines = append(lines, theme.Header.Render(" Add-ons"))
	lines = append(lines, "")

	// Syncthing summary
	lines = append(lines,
		" "+theme.Header.Render("🔄 Syncthing"))
	lines = append(lines, " "+theme.Dim.Render(
		"Auto-backup LND channel state"))
	if m.cfg.SyncthingInstalled {
		lines = append(lines,
			" "+theme.GreenDot.Render("●")+" "+
				theme.Good.Render("Installed")+
				"  "+theme.Dim.Render(
				fmt.Sprintf("%d paired",
					len(m.cfg.SyncthingDevices))))
	} else {
		lines = append(lines,
			" "+theme.RedDot.Render("●")+" "+
				theme.Dim.Render("Not installed"))
	}

	lines = append(lines, "")

	// LndHub summary
	lines = append(lines,
		" "+theme.Lightning.Render("⚡ LndHub"))
	lines = append(lines, " "+theme.Dim.Render(
		"Lightning accounts for family & friends"))
	if m.cfg.LndHubInstalled {
		activeCount := 0
		for _, a := range m.cfg.LndHubAccounts {
			if a.Active {
				activeCount++
			}
		}
		lines = append(lines,
			" "+theme.GreenDot.Render("●")+" "+
				theme.Good.Render("Installed")+
				"  "+theme.Dim.Render(
				fmt.Sprintf("%d active", activeCount)))
	} else {
		lines = append(lines,
			" "+theme.RedDot.Render("●")+" "+
				theme.Dim.Render("Not installed"))
	}

	lines = append(lines, "")
	lines = append(lines, m.addonsButtons())

	return strings.Join(lines, "\n")
}

func (m Model) addonsButtons() string {
	labels := []string{"Syncthing", "LndHub"}

	var parts []string
	for i, label := range labels {
		isActive := m.contentFocused && m.btnIdx == i
		if isActive {
			parts = append(parts,
				"▸ "+theme.BtnFocused.Render(label))
		} else {
			parts = append(parts,
				"  "+theme.BtnNormal.Render(label))
		}
	}

	return " " + strings.Join(parts, "  ")
}

// ── Syncthing flows ──────────────────────────────────────

func (m Model) syncthingDetailContent(
	w, h int,
) string {
	var lines []string
	lines = append(lines, theme.Header.Render(
		" 🔄 Syncthing — Details"))
	lines = append(lines, "")

	pairedCount := len(m.cfg.SyncthingDevices)
	lines = append(lines, " "+theme.Label.Render(
		fmt.Sprintf("Connections (%d paired)",
			pairedCount)))
	lines = append(lines, " "+theme.Dim.Render(
		strings.Repeat("─", w-4)))

	if pairedCount == 0 {
		lines = append(lines, " "+theme.Dim.Render(
			"No devices paired yet"))
	} else {
		for i, d := range m.cfg.SyncthingDevices {
			prefix := "  "
			style := theme.Value
			if m.syncCursor == i {
				prefix = "▸ "
				style = theme.Action
			}
			lines = append(lines,
				fmt.Sprintf(" %s%s %s  %s",
					prefix,
					theme.GreenDot.Render("●"),
					style.Render(d.Name),
					theme.Dim.Render(d.PairedAt)))
		}
	}

	vpsDeviceID := installer.GetSyncthingDeviceID()
	if vpsDeviceID != "" {
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Node ID:"))
		id := vpsDeviceID
		if len(id) > w-4 {
			id = id[:w-7] + "..."
		}
		lines = append(lines,
			" "+theme.Mono.Render(id))
	}

	lines = append(lines, "")

	// Buttons
	btnLabels := []string{
		"Pair Device", "Device QR", "Web UI",
	}
	var btnParts []string
	for i, label := range btnLabels {
		isActive := m.addonBtnIdx == i
		if isActive {
			btnParts = append(btnParts,
				"▸ "+theme.BtnFocused.Render(label))
		} else {
			btnParts = append(btnParts,
				"  "+theme.BtnNormal.Render(label))
		}
	}
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))

	return strings.Join(lines, "\n")
}

func (m Model) syncthingPairContent(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" Pair Device"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Device ID:"))
	lines = append(lines,
		" "+m.syncDeviceInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Paste your local Syncthing Device ID"))

	if m.syncPairError != "" {
		lines = append(lines, "")
		lines = append(lines, " "+theme.Warning.Render(
			"Error: "+m.syncPairError))
	}
	if m.syncPairSuccess {
		lines = append(lines, "")
		lines = append(lines, " "+theme.Success.Render(
			"✅ Device paired!"))
		vpsDeviceID := installer.GetSyncthingDeviceID()
		if vpsDeviceID != "" {
			lines = append(lines, "")
			lines = append(lines, " "+theme.Dim.Render(
				"Now add this node in local Syncthing:"))
			id := vpsDeviceID
			if len(id) > w-4 {
				id = id[:w-7] + "..."
			}
			lines = append(lines,
				" "+theme.Mono.Render(id))
		}
	}

	lines = append(lines, "")
	if m.syncPairSuccess {
		lines = append(lines, " "+theme.Dim.Render(
			"Enter done  Backspace back"))
	} else {
		lines = append(lines, " "+theme.Dim.Render(
			"Enter pair  Backspace cancel"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) syncthingWebUIContent(w int) string {
	var lines []string
	lines = append(lines, theme.Header.Render(
		" 🔄 Syncthing Web UI"))
	lines = append(lines, "")

	syncOnion := readOnion(paths.TorSyncthingHostname)
	if syncOnion == "" {
		lines = append(lines, " "+theme.Warn.Render(
			"Tor address not available yet."))
	} else {
		lines = append(lines,
			" "+theme.Label.Render("URL:"))
		url := "http://" + syncOnion + ":8384"
		if len(url) > w-4 {
			url = url[:w-7] + "..."
		}
		if m.showSecrets {
			lines = append(lines,
				" "+theme.Mono.Render(url))
		}

		btnLabel := "Full URL"
		if m.addonBtnIdx == 0 {
			lines = append(lines,
				" ▸ "+theme.BtnFocused.Render(
					btnLabel))
		} else {
			lines = append(lines,
				"   "+theme.BtnNormal.Render(
					btnLabel))
		}

		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("User: ")+
				theme.Mono.Render("admin"))
		if m.cfg.SyncthingPassword != "" {
			if m.showSecrets {
				lines = append(lines,
					" "+theme.Label.Render("Pass: ")+
						theme.Mono.Render(
							m.cfg.SyncthingPassword))

				btnHide := "Hide Password"
				if m.addonBtnIdx == 1 {
					lines = append(lines,
						" ▸ "+
							theme.BtnFocused.Render(
								btnHide))
				} else {
					lines = append(lines,
						"   "+
							theme.BtnNormal.Render(
								btnHide))
				}
			} else {
				lines = append(lines,
					" "+theme.Label.Render("Pass: ")+
						theme.Dim.Render("••••••••"))

				btnShow := "Show Password"
				if m.addonBtnIdx == 1 {
					lines = append(lines,
						" ▸ "+
							theme.BtnFocused.Render(
								btnShow))
				} else {
					lines = append(lines,
						"   "+
							theme.BtnNormal.Render(
								btnShow))
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) syncthingDeviceDetailContent(
	w int,
) string {
	var lines []string

	if m.syncCursor >= len(m.cfg.SyncthingDevices) {
		lines = append(lines,
			" "+theme.Warn.Render("Device not found"))
		return strings.Join(lines, "\n")
	}

	dev := m.cfg.SyncthingDevices[m.syncCursor]
	lines = append(lines,
		" "+theme.Header.Render(dev.Name))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Device ID:"))
	id := dev.DeviceID
	if len(id) > w-4 {
		id = id[:w-7] + "..."
	}
	lines = append(lines, " "+theme.Mono.Render(id))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Paired: ")+
			theme.Value.Render(dev.PairedAt))

	return strings.Join(lines, "\n")
}

func (m Model) syncthingPairedCount() int {
	return len(m.cfg.SyncthingDevices)
}

// ── LndHub flows ─────────────────────────────────────────

func (m Model) lndhubManageContent(w, h int) string {
	var lines []string
	lines = append(lines, theme.Lightning.Render(
		" ⚡ LndHub — Accounts"))
	lines = append(lines, "")

	accounts := m.cfg.LndHubAccounts
	if len(accounts) == 0 {
		lines = append(lines,
			" "+theme.Dim.Render("No accounts yet"))
	} else {
		visSize := h - 8
		if visSize < 5 {
			visSize = 5
		}
		viewStart := 0
		if m.hubCursor >= viewStart+visSize {
			viewStart = m.hubCursor - visSize + 1
		}
		viewEnd := viewStart + visSize
		if viewEnd > len(accounts) {
			viewEnd = len(accounts)
		}

		if viewStart > 0 {
			lines = append(lines,
				" "+theme.Dim.Render("  ↑ more"))
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
			lines = append(lines,
				fmt.Sprintf(" %s%s %s  %s",
					prefix, status,
					style.Render(a.Label),
					theme.Dim.Render(a.CreatedAt)))
		}
		if viewEnd < len(accounts) {
			lines = append(lines,
				" "+theme.Dim.Render("  ↓ more"))
		}
	}

	lines = append(lines, "")

	// Buttons
	btnLabels := []string{
		"Create", "Details", "Deactivate",
	}
	var btnParts []string
	for i, label := range btnLabels {
		isActive := m.addonBtnIdx == i
		if isActive {
			btnParts = append(btnParts,
				"▸ "+theme.BtnFocused.Render(label))
		} else {
			btnParts = append(btnParts,
				"  "+theme.BtnNormal.Render(label))
		}
	}
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))

	return strings.Join(lines, "\n")
}

func (m Model) lndhubCreateNameContent(w int) string {
	var lines []string
	lines = append(lines,
		theme.Header.Render(" Create Account"))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Name:"))
	lines = append(lines, " "+m.hubNameInput.View())
	lines = append(lines, "")
	lines = append(lines, " "+theme.Dim.Render(
		"Letters, numbers, spaces, hyphens"))
	lines = append(lines, " "+theme.Dim.Render(
		"Enter to create  Backspace to cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) lndhubCreatedContent(w int) string {
	var lines []string
	lines = append(lines, theme.Success.Render(
		" ✅ Account created: "+
			m.hubNameInput.Value()))
	lines = append(lines, "")

	if m.lastAccount != nil {
		hubOnion := readOnion(paths.TorLndHubHostname)
		if hubOnion != "" {
			lines = append(lines,
				" "+theme.Label.Render("Tor:"))
			tor := hubOnion + ":" +
				paths.LndHubExternalPort
			if len(tor) > w-4 {
				tor = tor[:w-7] + "..."
			}
			lines = append(lines,
				" "+theme.Mono.Render(tor))
		}
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Label.Render("Login:    ")+
				theme.Mono.Render(
					m.lastAccount.Login))
		lines = append(lines,
			" "+theme.Label.Render("Password: ")+
				theme.Mono.Render(
					m.lastAccount.Password))
		lines = append(lines, "")
		lines = append(lines, " "+theme.Warning.Render(
			"Share with "+m.hubNameInput.Value()+
				". Won't be shown again."))

		lines = append(lines, "")

		qrLabel := "Show QR"
		if m.addonBtnIdx == 0 {
			lines = append(lines,
				" ▸ "+theme.BtnFocused.Render(
					qrLabel))
		} else {
			lines = append(lines,
				"   "+theme.BtnNormal.Render(
					qrLabel))
		}
	}

	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Dim.Render("Enter done"))
	return strings.Join(lines, "\n")
}

func (m Model) lndhubAccountDetailContent(
	w int,
) string {
	var lines []string

	if m.hubCursor >= len(m.cfg.LndHubAccounts) {
		lines = append(lines,
			" "+theme.Warn.Render("Account not found"))
		return strings.Join(lines, "\n")
	}

	acct := m.cfg.LndHubAccounts[m.hubCursor]
	lines = append(lines,
		" "+theme.Header.Render(acct.Label))
	lines = append(lines, "")
	lines = append(lines,
		" "+theme.Label.Render("Login:   ")+
			theme.Mono.Render(acct.Login))
	lines = append(lines,
		" "+theme.Label.Render("Created: ")+
			theme.Value.Render(acct.CreatedAt))

	if acct.Active {
		lines = append(lines,
			" "+theme.Label.Render("Status:  ")+
				theme.Success.Render("active"))
	} else {
		lines = append(lines,
			" "+theme.Label.Render("Status:  ")+
				theme.Warning.Render("deactivated"))
		if acct.DeactivatedAt != "" {
			lines = append(lines,
				" "+theme.Label.Render(
					"Deactivated: ")+
					theme.Value.Render(
						acct.DeactivatedAt))
		}
		if acct.BalanceOnDeactivate != "" &&
			acct.BalanceOnDeactivate != "0" &&
			acct.BalanceOnDeactivate != "unknown" {
			lines = append(lines, "")
			lines = append(lines,
				" "+theme.Warning.Render(
					"Had "+
						acct.BalanceOnDeactivate+
						" sats"))
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) lndhubDeactivateContent(w int) string {
	var lines []string

	if m.hubCursor < len(m.cfg.LndHubAccounts) {
		acct := m.cfg.LndHubAccounts[m.hubCursor]
		lines = append(lines, " "+theme.Warning.Render(
			"Deactivate "+acct.Label+"?"))
		lines = append(lines, "")
		lines = append(lines, " "+theme.Value.Render(
			"• Block wallet access"))
		lines = append(lines, " "+theme.Value.Render(
			"• Record balance"))
		lines = append(lines, " "+theme.Value.Render(
			"• Login stops working"))
		lines = append(lines, "")
		lines = append(lines, " "+theme.Dim.Render(
			"y confirm  n cancel"))
	}

	return strings.Join(lines, "\n")
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
			"Scan → paste into local Syncthing"))
	}

	lines = append(lines, "")
	lines = append(lines, theme.Footer.Render(
		"backspace back • q quit"))
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Left, lines...))
}
