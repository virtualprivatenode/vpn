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

// ── Add-ons overview (card buttons) ──────────────────────

func (m Model) addonsOverview(w, h int) string {
	var lines []string
	lines = append(lines, "")

	boxW := w - 6
	if boxW < 28 {
		boxW = 28
	}

	isFocused := m.contentFocused && !m.tabFocused

	borderNormal := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	borderActive := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220"))

	titleNormal := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true)
	titleActive := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Bold(true)

	// Helper to render a card
	renderCard := func(
		icon, name, desc string,
		statusLine1, statusLine2 string,
		selected bool,
	) {
		brd := borderNormal
		ttl := titleNormal
		if selected {
			brd = borderActive
			ttl = titleActive
		}

		// Card is 8 lines tall:
		// border, blank, title, desc, blank,
		// status1, status2, border
		cardH := 8
		markerRow := cardH / 2

		topLine := brd.Render(
			"┌" + strings.Repeat("─", boxW-2) + "┐")
		botLine := brd.Render(
			"└" + strings.Repeat("─", boxW-2) + "┘")

		cardRows := make([]string, cardH)
		cardRows[0] = topLine
		cardRows[cardH-1] = botLine

		innerLines := []string{
			"",
			icon + " " + ttl.Render(name),
			theme.Dim.Render(desc),
			"",
			statusLine1,
			statusLine2,
		}

		for i := 1; i < cardH-1; i++ {
			content := ""
			idx := i - 1
			if idx < len(innerLines) {
				content = innerLines[idx]
			}
			contentVis := lipgloss.Width(content)
			padR := boxW - 2 - contentVis - 1
			if padR < 0 {
				padR = 0
			}
			cardRows[i] = brd.Render("│") +
				" " + content +
				strings.Repeat(" ", padR) +
				brd.Render("│")
		}

		for i, row := range cardRows {
			marker := "  "
			if selected && i == markerRow {
				marker = navActiveStyle.Render("▸") +
					" "
			}
			lines = append(lines, marker+row)
		}
	}

	// ── Syncthing card ───────────────────────────
	syncSelected := isFocused && m.btnIdx == 0

	var syncStat1, syncStat2 string
	if m.cfg.SyncthingInstalled {
		syncStat1 = theme.GreenDot.Render("●") +
			" " + theme.Good.Render("Installed")
		syncStat2 = fmt.Sprintf(
			"%d paired",
			len(m.cfg.SyncthingDevices))
		syncStat2 = theme.Dim.Render(syncStat2)
	} else {
		syncStat1 = theme.RedDot.Render("●") +
			" " + theme.Dim.Render("Not installed")
		syncStat2 = ""
	}

	renderCard(
		"🔄", "Syncthing",
		"Auto-backup LND channel state",
		syncStat1, syncStat2,
		syncSelected,
	)

	lines = append(lines, "")

	// ── LndHub card ──────────────────────────────
	hubSelected := isFocused && m.btnIdx == 1

	var hubStat1, hubStat2 string
	if m.cfg.LndHubInstalled {
		activeCount := 0
		for _, a := range m.cfg.LndHubAccounts {
			if a.Active {
				activeCount++
			}
		}
		hubStat1 = theme.GreenDot.Render("●") +
			" " + theme.Good.Render("Installed")
		hubStat2 = theme.Dim.Render(
			fmt.Sprintf("%d active", activeCount))
	} else {
		hubStat1 = theme.RedDot.Render("●") +
			" " + theme.Dim.Render("Not installed")
		hubStat2 = ""
	}

	renderCard(
		"⚡", "LndHub",
		"Lightning accounts for family & friends",
		hubStat1, hubStat2,
		hubSelected,
	)

	return strings.Join(lines, "\n")
}

func (m Model) addonsButtons() string {
	return ""
}

// ── Syncthing flows ──────────────────────────────────────

func (m Model) syncthingDetailContent(
	w, h int,
) string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, theme.Header.Render(
		" 🔄 Syncthing — Details"))
	lines = append(lines, "")

	// ── Buttons (full width, above table) ────────
	btnLabels := []string{
		"Pair Device", "Device QR", "Web UI",
	}
	btnW := w - 2
	if btnW < 20 {
		btnW = 20
	}
	numBtns := len(btnLabels)
	gaps := numBtns - 1
	totalGap := gaps * 2
	perBtn := (btnW - totalGap) / numBtns
	if perBtn < 8 {
		perBtn = 8
	}

	isFocused := m.contentFocused && !m.tabFocused

	var btnParts []string
	for i, label := range btnLabels {
		isActive := isFocused && m.addonBtnIdx == i
		if isActive {
			btnParts = append(btnParts,
				theme.BtnFocused.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		} else {
			btnParts = append(btnParts,
				theme.BtnNormal.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		}
	}
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))
	lines = append(lines, "")
	lines = append(lines, "")

	// ── Devices table ────────────────────────────
	pairedCount := len(m.cfg.SyncthingDevices)
	lines = append(lines, " "+theme.Label.Render(
		fmt.Sprintf("Paired Devices (%d)",
			pairedCount)))
	lines = append(lines, "")

	if pairedCount == 0 {
		lines = append(lines, " "+theme.Dim.Render(
			"No devices paired yet"))
	} else {
		hdrStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Bold(true)
		sepStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

		nameW := 20
		idW := 24
		dateW := w - nameW - idW - 6
		if dateW < 12 {
			dateW = 12
		}

		hdr := " " +
			hdrStyle.Render(pad("Name", nameW)) +
			hdrStyle.Render(pad("Device ID", idW)) +
			hdrStyle.Render(
				fmt.Sprintf("%-*s", dateW, "Paired"))
		lines = append(lines, hdr)
		lines = append(lines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		maxRows := h - 14
		if maxRows < 3 {
			maxRows = 3
		}

		startIdx := 0
		if m.syncCursor >= startIdx+maxRows {
			startIdx = m.syncCursor - maxRows + 1
		}
		endIdx := startIdx + maxRows
		if endIdx > pairedCount {
			endIdx = pairedCount
		}

		selStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

		if startIdx > 0 {
			lines = append(lines,
				" "+theme.Dim.Render("  ↑ more"))
		}

		for i := startIdx; i < endIdx; i++ {
			d := m.cfg.SyncthingDevices[i]

			name := d.Name
			if len(name) > nameW-1 {
				name = name[:nameW-2] + ".."
			}
			nameStr := pad(name, nameW)

			devID := d.DeviceID
			if len(devID) > idW-1 {
				devID = devID[:idW-4] + "..."
			}
			idStr := pad(devID, idW)

			dateStr := fmt.Sprintf("%-*s",
				dateW, d.PairedAt)

			isSelected := isFocused &&
				m.syncCursor == i

			marker := " "
			if isSelected {
				marker = "▸"
				lines = append(lines,
					marker+
						selStyle.Render(nameStr)+
						selStyle.Render(idStr)+
						selStyle.Render(dateStr))
			} else {
				lines = append(lines,
					marker+
						theme.Value.Render(nameStr)+
						theme.Dim.Render(idStr)+
						theme.Dim.Render(dateStr))
			}
		}

		if endIdx < pairedCount {
			lines = append(lines,
				" "+theme.Dim.Render("  ↓ more"))
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

	return strings.Join(lines, "\n")
}

func (m Model) syncthingPairContent(w int) string {
	var lines []string
	lines = append(lines, "")
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
	lines = append(lines, "")
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
	lines = append(lines, "")
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
	lines = append(lines, "")
	lines = append(lines, theme.Lightning.Render(
		" ⚡ LndHub — Accounts"))
	lines = append(lines, "")

	// ── Buttons (full width, above table) ────────
	btnLabels := []string{
		"Create", "Details", "Deactivate",
	}
	btnW := w - 2
	if btnW < 20 {
		btnW = 20
	}
	numBtns := len(btnLabels)
	gaps := numBtns - 1
	totalGap := gaps * 2
	perBtn := (btnW - totalGap) / numBtns
	if perBtn < 8 {
		perBtn = 8
	}

	isFocused := m.contentFocused && !m.tabFocused

	var btnParts []string
	for i, label := range btnLabels {
		isActive := isFocused && m.addonBtnIdx == i
		if isActive {
			btnParts = append(btnParts,
				theme.BtnFocused.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		} else {
			btnParts = append(btnParts,
				theme.BtnNormal.
					Width(perBtn).
					AlignHorizontal(
						lipgloss.Center).
					Render(label))
		}
	}
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))
	lines = append(lines, "")
	lines = append(lines, "")

	// ── Accounts table ───────────────────────────
	accounts := m.cfg.LndHubAccounts
	if len(accounts) == 0 {
		lines = append(lines,
			" "+theme.Dim.Render("No accounts yet"))
	} else {
		hdrStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Bold(true)
		sepStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

		nameW := 18
		loginW := 16
		statusW := 12
		dateW := w - nameW - loginW - statusW - 6
		if dateW < 12 {
			dateW = 12
		}

		hdr := " " +
			hdrStyle.Render(pad("Name", nameW)) +
			hdrStyle.Render(pad("Login", loginW)) +
			hdrStyle.Render(
				pad("Status", statusW)) +
			hdrStyle.Render(
				fmt.Sprintf("%-*s", dateW, "Created"))
		lines = append(lines, hdr)
		lines = append(lines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		visSize := h - 12
		if visSize < 3 {
			visSize = 3
		}
		viewStart := 0
		if m.hubCursor >= viewStart+visSize {
			viewStart = m.hubCursor - visSize + 1
		}
		viewEnd := viewStart + visSize
		if viewEnd > len(accounts) {
			viewEnd = len(accounts)
		}

		selStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

		if viewStart > 0 {
			lines = append(lines,
				" "+theme.Dim.Render("  ↑ more"))
		}

		for i := viewStart; i < viewEnd; i++ {
			a := accounts[i]

			name := a.Label
			if len(name) > nameW-1 {
				name = name[:nameW-2] + ".."
			}
			nameStr := pad(name, nameW)

			login := a.Login
			if len(login) > loginW-1 {
				login = login[:loginW-2] + ".."
			}
			loginStr := pad(login, loginW)

			var statusStr string
			if a.Active {
				statusStr = pad("● active", statusW)
			} else {
				statusStr = pad("● off", statusW)
			}

			dateStr := fmt.Sprintf("%-*s",
				dateW, a.CreatedAt)

			isSelected := isFocused &&
				m.hubCursor == i

			marker := " "
			if isSelected {
				marker = "▸"
				lines = append(lines,
					marker+
						selStyle.Render(nameStr)+
						selStyle.Render(loginStr)+
						selStyle.Render(statusStr)+
						selStyle.Render(dateStr))
			} else {
				var stRendered string
				if a.Active {
					stRendered = theme.Good.Render(
						statusStr)
				} else {
					stRendered = theme.Warn.Render(
						statusStr)
				}
				lines = append(lines,
					marker+
						theme.Value.Render(nameStr)+
						theme.Dim.Render(loginStr)+
						stRendered+
						theme.Dim.Render(dateStr))
			}
		}

		if viewEnd < len(accounts) {
			lines = append(lines,
				" "+theme.Dim.Render("  ↓ more"))
		}
	}

	return strings.Join(lines, "\n")
}

func (m Model) lndhubCreateNameContent(w int) string {
	var lines []string
	lines = append(lines, "")
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
	lines = append(lines, "")
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
	lines = append(lines, "")
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
