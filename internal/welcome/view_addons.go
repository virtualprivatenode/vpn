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
	isFocused := m.contentFocused && !m.tabFocused

	boxW := w - 6
	if boxW < 28 {
		boxW = 28
	}

	borderNormal := theme.AddonBorderNormal
	borderActive := theme.AddonBorderActive

	titleNormal := theme.AddonTitleNormal
	titleActive := theme.AddonTitleActive

	// ── Scrollable middle (all cards) ────────────
	var midLines []string

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
				marker =
					theme.NavActive.Render("▸") + " "
			}
			midLines = append(midLines, marker+row)
		}
	}

	syncSelected := isFocused && m.btnIdx == 0

	var syncStat1, syncStat2 string
	if m.cfg.SyncthingInstalled {
		syncStat1 = theme.GreenDot.Render("●") +
			" " + theme.Good.Render("Installed")
		syncStat2 = theme.Dim.Render(fmt.Sprintf(
			"%d paired",
			len(m.cfg.SyncthingDevices)))
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

	midLines = append(midLines, "")

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

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ─────────────────────────────────
	// Top gap line
	vpH := h - 1
	if vpH < 1 {
		vpH = 1
	}

	// Each card is 8 lines + 1 gap = 9 lines
	cursorLine := m.btnIdx * 9

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines), isFocused)

	// ── Assemble output ──────────────────────────
	return "\n" + vpRendered
}

// ── Syncthing detail ─────────────────────────────────────

func (m Model) syncthingDetailContent(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "🔄 Syncthing — Details")

	isFocused := m.contentFocused && !m.tabFocused
	onButtons := isFocused && m.addonFocus == 0

	p.buttons(
		[]string{"Pair Device", "Device QR", "Web UI"},
		m.addonBtnIdx, onButtons)
	p.blank()
	p.blank()

	pairedCount := len(m.cfg.SyncthingDevices)
	p.labelLine(fmt.Sprintf(
		"Paired Devices (%d)", pairedCount))
	p.blank()

	if pairedCount == 0 {
		p.dim("No devices paired yet")
	} else {
		hdrStyle := theme.TableHeader
		sepStyle := theme.TableDim

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
		p.line(hdr)
		p.line(" " + sepStyle.Render(
			strings.Repeat("─", w-2)))

		onList := isFocused && m.addonFocus == 1
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
			Foreground(theme.ColorAccent).
			Bold(true)

		if startIdx > 0 {
			p.dim("  ↑ more")
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

			isSelected := onList && m.syncCursor == i

			marker := " "
			if isSelected {
				marker = "▸"
				p.line(marker +
					selStyle.Render(nameStr) +
					selStyle.Render(idStr) +
					selStyle.Render(dateStr))
			} else {
				p.line(marker +
					theme.Value.Render(nameStr) +
					theme.Dim.Render(idStr) +
					theme.Dim.Render(dateStr))
			}
		}

		if endIdx < pairedCount {
			p.dim("  ↓ more")
		}
	}

	vpsDeviceID := installer.GetSyncthingDeviceID()
	if vpsDeviceID != "" {
		p.blank()
		p.labelLine("Node ID:")
		id := vpsDeviceID
		if len(id) > w-4 {
			id = id[:w-7] + "..."
		}
		p.mono(id)
	}

	return p.render()
}

func (m Model) syncthingPairContent(w int) string {
	p := newPane(w)
	p.title(theme.Header, "Pair Device")

	p.input("Device ID:",
		m.syncDeviceInput,
		m.contentFocused)
	p.blank()
	p.dim("Paste your local Syncthing Device ID.")
	p.dim("Find it in Syncthing → Actions → Show ID")
	p.dim("Format: XXXXXXX-XXXXXXX-XXXXXXX-...")
	p.blank()
	p.dim("After pairing, add this node's ID in your")
	p.dim("local Syncthing to complete the connection.")

	if m.syncPairError != "" {
		p.blank()
		p.warn("Error: " + m.syncPairError)
	}
	if m.syncPairSuccess {
		p.blank()
		p.success("✅ Device paired!")
		vpsDeviceID := installer.GetSyncthingDeviceID()
		if vpsDeviceID != "" {
			p.blank()
			p.dim(
				"Now add this node in your " +
					"local Syncthing:")
			id := vpsDeviceID
			if len(id) > w-4 {
				id = id[:w-7] + "..."
			}
			p.mono(id)
		}
	}

	return p.render()
}

func (m Model) syncthingWebUIContent(w int) string {
	p := newPane(w)
	p.title(theme.Header, "🔄 Syncthing Web UI")

	syncOnion := readOnion(paths.TorSyncthingHostname)
	if syncOnion == "" {
		p.warn("Tor address not available yet.")
		return p.render()
	}

	isFocused := m.contentFocused && !m.tabFocused

	url := "http://" + syncOnion + ":8384"
	if len(url) > w-4 {
		url = url[:w-7] + "..."
	}

	p.labelLine("URL:")
	if m.showSecrets {
		p.mono(url)
	}
	p.blank()
	p.monoField("User: ", "admin")

	if m.cfg.SyncthingPassword != "" {
		if m.showSecrets {
			p.monoField("Pass: ",
				m.cfg.SyncthingPassword)
		} else {
			p.line(" " +
				theme.Label.Render("Pass: ") +
				theme.Dim.Render("••••••••"))
		}
	}
	p.blank()

	showLabel := "Show Password"
	if m.showSecrets {
		showLabel = "Hide Password"
	}
	p.buttons(
		[]string{"Full URL", showLabel},
		m.addonBtnIdx, isFocused)

	return p.render()
}

func (m Model) syncthingDeviceDetailContent(
	w int,
) string {
	if m.syncCursor >= len(m.cfg.SyncthingDevices) {
		p := newPane(w)
		p.warn("Device not found")
		return p.render()
	}

	dev := m.cfg.SyncthingDevices[m.syncCursor]
	p := newPane(w)
	p.title(theme.Header, dev.Name)

	p.labelLine("Device ID:")
	id := dev.DeviceID
	if len(id) > w-4 {
		id = id[:w-7] + "..."
	}
	p.mono(id)
	p.blank()
	p.field("Paired: ", dev.PairedAt)

	return p.render()
}

// ── LndHub flows ─────────────────────────────────────────

func (m Model) lndhubManageContent(
	w, h int,
) string {
	p := newPane(w)
	p.title(theme.Header, "⚡ LndHub — Accounts")

	isFocused := m.contentFocused && !m.tabFocused
	onButtons := isFocused && m.addonFocus == 0

	p.buttons(
		[]string{"Create", "Details", "Deactivate"},
		m.addonBtnIdx, onButtons)
	p.blank()
	p.blank()

	accounts := m.cfg.LndHubAccounts
	if len(accounts) == 0 {
		p.dim("No accounts yet")
	} else {
		hdrStyle := theme.TableHeader
		sepStyle := theme.TableDim

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
		p.line(hdr)
		p.line(" " + sepStyle.Render(
			strings.Repeat("─", w-2)))

		onList := isFocused && m.addonFocus == 1
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
			Foreground(theme.ColorAccent).
			Bold(true)

		if viewStart > 0 {
			p.dim("  ↑ more")
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
				statusStr = pad(
					"● active", statusW)
			} else {
				statusStr = pad("● off", statusW)
			}

			dateStr := fmt.Sprintf("%-*s",
				dateW, a.CreatedAt)

			isSelected := onList && m.hubCursor == i

			marker := " "
			if isSelected {
				marker = "▸"
				p.line(marker +
					selStyle.Render(nameStr) +
					selStyle.Render(loginStr) +
					selStyle.Render(statusStr) +
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
				p.line(marker +
					theme.Value.Render(nameStr) +
					theme.Dim.Render(loginStr) +
					stRendered +
					theme.Dim.Render(dateStr))
			}
		}

		if viewEnd < len(accounts) {
			p.dim("  ↓ more")
		}
	}

	return p.render()
}

func (m Model) lndhubCreateNameContent(w int) string {
	p := newPane(w)
	p.title(theme.Header, "Create Account")

	p.dim("Create a custodial Lightning wallet account.")
	p.dim("The recipient will receive a login and")
	p.dim("password to connect via BlueWallet or Zeus.")
	p.blank()

	p.input("Name:", m.hubNameInput, m.contentFocused)
	p.blank()
	p.dim("Letters, numbers, spaces, hyphens")

	return p.render()
}

func (m Model) lndhubCreatedContent(w int) string {
	p := newPane(w)
	p.title(theme.Success,
		"✅ Account created: "+
			m.hubNameInput.Value())

	if m.lastAccount != nil {
		hubOnion := readOnion(paths.TorLndHubHostname)
		if hubOnion != "" {
			p.labelLine("Tor:")
			tor := hubOnion + ":" +
				paths.LndHubExternalPort
			if len(tor) > w-4 {
				tor = tor[:w-7] + "..."
			}
			p.mono(tor)
		}
		p.blank()
		p.monoField("Login:    ",
			m.lastAccount.Login)
		p.monoField("Password: ",
			m.lastAccount.Password)
		p.blank()
		p.warn("Share with " +
			m.hubNameInput.Value() +
			". Won't be shown again.")
		p.blank()

		isFocused := m.contentFocused && !m.tabFocused
		p.buttons(
			[]string{"Show QR", "Done"},
			m.addonBtnIdx, isFocused)
	}

	return p.render()
}

func (m Model) lndhubAccountDetailContent(
	w int,
) string {
	if m.hubCursor >= len(m.cfg.LndHubAccounts) {
		p := newPane(w)
		p.warn("Account not found")
		return p.render()
	}

	acct := m.cfg.LndHubAccounts[m.hubCursor]
	p := newPane(w)
	p.title(theme.Header, acct.Label)

	p.monoField("Login:   ", acct.Login)
	p.field("Created: ", acct.CreatedAt)

	if acct.Active {
		p.line(" " + theme.Label.Render("Status:  ") +
			theme.Success.Render("active"))
	} else {
		p.line(" " + theme.Label.Render("Status:  ") +
			theme.Warning.Render("deactivated"))
		if acct.DeactivatedAt != "" {
			p.field("Deactivated: ",
				acct.DeactivatedAt)
		}
		if acct.BalanceOnDeactivate != "" &&
			acct.BalanceOnDeactivate != "0" &&
			acct.BalanceOnDeactivate != "unknown" {
			p.blank()
			p.warn("Had " +
				acct.BalanceOnDeactivate + " sats")
		}
	}

	return p.render()
}

func (m Model) lndhubDeactivateContent(
	w int,
) string {
	p := newPane(w)

	if m.hubCursor < len(m.cfg.LndHubAccounts) {
		acct := m.cfg.LndHubAccounts[m.hubCursor]
		p.title(theme.Warning,
			"Deactivate "+acct.Label+"?")
		p.line(" " + theme.Value.Render(
			"• Block wallet access"))
		p.line(" " + theme.Value.Render(
			"• Record balance"))
		p.line(" " + theme.Value.Render(
			"• Login stops working"))
	}

	return p.render()
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
		lipgloss.JoinVertical(
			lipgloss.Left, lines...))
}
