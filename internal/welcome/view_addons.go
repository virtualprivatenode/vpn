package welcome

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) renderLndHubTabContent(
	w, h int,
) string {
	switch m.subview {
	case svLndHubCreateName:
		return m.lndhubCreateNameContent(w)
	case svLndHubCreateAccount:
		return m.lndhubCreatedContent(w)
	case svLndHubCreateQR:
		return m.lndhubCreateQRContent(w)
	default:
		return m.lndhubManageContent(w, h)
	}
}

// ── Add-ons overview ─────────────────────────────────────

func (m Model) addonsOverview(w, h int) string {
	isFocused := m.contentFocused && !m.tabFocused

	titleNormal := theme.AddonTitleNormal
	titleActive := theme.AddonTitleActive
	sepStyle := theme.TableDim

	renderSection := func(
		icon, name, desc string,
		statusLine1, statusLine2 string,
		selected bool,
	) []string {
		ttl := titleNormal
		if selected {
			ttl = titleActive
		}

		marker := "  "
		if selected {
			marker =
				theme.NavActive.Render("▸") + " "
		}

		var lines []string
		lines = append(lines,
			marker+icon+" "+ttl.Render(name))
		lines = append(lines, "")
		lines = append(lines,
			"   "+theme.Dim.Render(desc))
		lines = append(lines, "")
		lines = append(lines,
			"   "+statusLine1)
		if statusLine2 != "" {
			lines = append(lines,
				"   "+statusLine2)
		}
		return lines
	}

	// ── Syncthing section content ────────────────
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

	syncLines := renderSection(
		"🔄", "Syncthing",
		"Auto-backup LND channel state",
		syncStat1, syncStat2,
		syncSelected,
	)

	// ── LndHub section content ───────────────────
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

	hubLines := renderSection(
		"⚡", "LndHub",
		"Lightning accounts for family & friends",
		hubStat1, hubStat2,
		hubSelected,
	)

	// ── Layout: two halves + divider ─────────────
	// 1 line for divider, remaining split evenly
	bodyH := h
	if bodyH < 4 {
		bodyH = 4
	}

	topH := (bodyH - 1) / 2
	botH := bodyH - 1 - topH

	// Center section content vertically in its half
	centerInHalf := func(
		content []string, halfH int,
	) []string {
		pad := (halfH - len(content)) / 2
		if pad < 0 {
			pad = 0
		}
		blank := ""
		var out []string
		for i := 0; i < pad; i++ {
			out = append(out, blank)
		}
		out = append(out, content...)
		for len(out) < halfH {
			out = append(out, blank)
		}
		return out
	}

	var lines []string

	// Top half: Syncthing
	lines = append(lines,
		centerInHalf(syncLines, topH)...)

	// Divider — full width
	lines = append(lines,
		sepStyle.Render(
			strings.Repeat("─", w)))

	// Bottom half: LndHub
	lines = append(lines,
		centerInHalf(hubLines, botH)...)

	return strings.Join(lines, "\n")
}

// ── LndHub flows ─────────────────────────────────────────

func (m Model) lndhubManageContent(
	w, h int,
) string {
	isFocused := m.contentFocused && !m.tabFocused
	onButtons := isFocused && m.contentFocus() == 0

	// ── Fixed header: title + button ─────────────
	var headerLines []string
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		centerPad(
			theme.Header.Render(
				"LndHub Accounts"), w))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines,
		renderButtons(
			[]string{"Create New Account"},
			m.addonBtnIdx, onButtons, w))
	headerLines = append(headerLines, "")

	headerH := len(headerLines)
	header := strings.Join(headerLines, "\n")

	// ── Scrollable body ─────────────────────────
	var midLines []string
	cursorLine := 0

	accounts := m.cfg.LndHubAccounts
	if len(accounts) == 0 {
		midLines = append(midLines,
			" "+theme.Dim.Render("No accounts yet"))
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
		midLines = append(midLines, hdr)
		midLines = append(midLines,
			" "+sepStyle.Render(
				strings.Repeat("─", w-2)))

		onList := isFocused && m.contentFocus() == 1

		selStyle := lipgloss.NewStyle().
			Foreground(theme.ColorAccent).
			Bold(true)

		tableStart := len(midLines)

		for i, a := range accounts {
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
				cursorLine = tableStart + i
				midLines = append(midLines,
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
				midLines = append(midLines,
					marker+
						theme.Value.Render(nameStr)+
						theme.Dim.Render(loginStr)+
						stRendered+
						theme.Dim.Render(dateStr))
			}
		}
	}

	midContent := strings.Join(midLines, "\n")

	// ── Viewport ────────────────────────────────
	vpH := h - headerH
	if vpH < 1 {
		vpH = 1
	}

	vpRendered := renderViewport(
		midContent, w, vpH, cursorLine,
		len(midLines),
		isFocused && m.contentFocus() == 1)

	return header + "\n" + vpRendered
}

func (m Model) lndhubCreateNameContent(w int) string {
	p := newPane(w)
	p.title(theme.Header, "Create New Account")

	p.dim("Create a custodial Lightning wallet account.")
	p.dim("The recipient will receive a login and")
	p.dim("password to connect via BlueWallet or Zeus.")
	p.blank()

	inputFocused := m.contentFocused &&
		!m.tabFocused && m.contentFocus() == 0
	p.input("Name:", m.hubNameInput, inputFocused)
	p.blank()
	p.dim("Letters, numbers, spaces, hyphens")

	p.blank()
	btnFocused := m.contentFocused &&
		!m.tabFocused && m.contentFocus() == 1
	p.buttons(
		[]string{"Clear", "Create Account"},
		m.hubCreateBtnIdx, btnFocused)

	return p.render()
}

func (m Model) lndhubCreatedContent(w int) string {
	p := newPane(w)
	p.title(theme.Success,
		"Account created: "+
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
		buttons := []string{"Done"}
		if hubOnion != "" {
			buttons = []string{"Show QR", "Done"}
		}
		p.line(renderButtons(
			buttons,
			m.addonBtnIdx, isFocused, w))
	}

	return p.render()
}

func (m Model) lndhubCreateQRContent(w int) string {
	p := newPane(w)
	p.title(theme.Header, "LndHub Connection QR")

	if m.lastAccount != nil {
		hubOnion := readOnion(paths.TorLndHubHostname)
		if hubOnion != "" {
			qrData := fmt.Sprintf(
				"lndhub://%s:%s@%s:%s",
				m.lastAccount.Login,
				m.lastAccount.Password,
				hubOnion,
				paths.LndHubExternalPort)
			qr := renderQRCode(qrData)
			if qr != "" {
				p.dim("Scan with BlueWallet or Zeus")
				p.blank()
				for _, line := range strings.Split(
					qr, "\n") {
					lineW := lipgloss.Width(line)
					padN := (w - lineW) / 2
					if padN < 0 {
						padN = 0
					}
					p.line(
						strings.Repeat(" ", padN) +
							line)
				}
			}
		}
	}

	p.blank()
	isFocused := m.contentFocused && !m.tabFocused
	p.buttons(
		[]string{"Back"},
		0, isFocused)

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

		// Deactivate button (like Close Channel)
		p.blank()
		isFocused := m.contentFocused &&
			!m.tabFocused
		isOnButton := isFocused &&
			m.contentFocus() == 1
		p.line(renderButtons(
			[]string{"Deactivate"},
			0, isOnButton, w))
	} else {
		p.line(" " + theme.Label.Render("Status:  ") +
			theme.Warning.Render("deactivated"))
		if acct.DeactivatedAt != "" {
			p.field("Deactivated: ",
				acct.DeactivatedAt)
		}
		if acct.BalanceOnDeactivate != "" {
			p.field("Balance:     ",
				acct.BalanceOnDeactivate+" sats")
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

	p.blank()

	isFocused := m.contentFocused && !m.tabFocused
	p.line(renderButtons(
		[]string{"Go Back", "Deactivate"},
		m.hubDeactivateBtnIdx, isFocused, w))

	return p.render()
}
