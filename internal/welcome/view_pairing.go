package welcome

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	qrterminal "github.com/mdp/qrterminal/v3"

	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) pairingContent(w, h int) string {
	if !m.cfg.HasLND() || !m.cfg.WalletExists() {
		p := newPane(w)
		p.title(theme.Lightning, "⚡ Zeus Wallet")
		if m.cfg.HasLND() {
			p.dim("Create LND wallet first")
		} else {
			p.dim("Install LND first")
		}
		return p.render()
	}

	if m.status == nil || !m.status.lndResponding {
		p := newPane(w)
		p.title(theme.Lightning, "⚡ Zeus Wallet")
		p.dim("Waiting for LND...")
		return p.render()
	}

	isFocused := m.contentFocused && !m.tabFocused

	p := newPane(w)
	p.title(theme.Lightning, "⚡ Zeus — LND REST")

	restOnion := readOnion(paths.TorLNDRESTHostname)

	if m.cfg.P2PMode == "hybrid" {
		p.line(" " + theme.Header.Render(
			"🛜 Clearnet"))
		if m.status != nil &&
			m.status.publicIP != "" {
			p.monoField("Server: ",
				m.status.publicIP)
			p.monoField("Port:   ", "8080")
		}
		p.blank()
		p.line(" " + theme.Header.Render("🧅 Tor"))
	}

	if restOnion == "" {
		p.warn("Tor not available")
	} else {
		server := restOnion
		if len(server) > w-14 {
			server = server[:w-17] + "..."
		}
		p.monoField("Server: ", server)
		p.monoField("Port:   ", "8080")
	}

	mac := readMacaroonHex(m.cfg)
	if mac != "" {
		p.blank()
		preview := mac[:min(24, len(mac))] + "..."
		p.monoField("Macaroon: ", preview)
	}

	p.blank()

	btnLabels := []string{"QR (Tor)", "Macaroon"}
	if m.cfg.P2PMode == "hybrid" {
		btnLabels = append(btnLabels, "QR (Clearnet)")
	}
	p.buttons(btnLabels, m.pairingButtonIdx, isFocused)

	return p.render()
}

func (m Model) handlePairingEnter() (
	tea.Model, tea.Cmd,
) {
	switch m.pairingButtonIdx {
	case 0:
		restOnion := readOnion(
			paths.TorLNDRESTHostname)
		mac := readMacaroonHex(m.cfg)
		if restOnion != "" && mac != "" {
			m.urlTarget = fmt.Sprintf(
				"lndconnect://%s:8080?macaroon=%s",
				restOnion, hexToBase64URL(mac))
			m.qrLabel = "Tor QR — " +
				restOnion[:min(20,
					len(restOnion))] + "..."
			m.qrMode = "tor"
			m.urlReturnTo = svWalletPairing
			m.subview = svQR
		}
	case 1:
		return m, showMacaroonCmd(m.cfg)
	case 2:
		if m.cfg.P2PMode == "hybrid" &&
			m.status != nil &&
			m.status.publicIP != "" {
			mac := readMacaroonHex(m.cfg)
			if mac != "" {
				m.urlTarget = fmt.Sprintf(
					"lndconnect://%s:8080"+
						"?macaroon=%s",
					m.status.publicIP,
					hexToBase64URL(mac))
				m.qrLabel = "Clearnet QR — " +
					m.status.publicIP + ":8080"
				m.qrMode = "clearnet"
				m.urlReturnTo = svWalletPairing
				m.subview = svQR
			}
		}
	}
	return m, nil
}

func (m Model) viewQR() string {
	var uri string
	var label string

	if m.qrLabel != "" {
		uri = m.urlTarget
		label = m.qrLabel
	} else {
		restOnion := readOnion(
			paths.TorLNDRESTHostname)
		mac := readMacaroonHex(m.cfg)

		if m.qrMode == "clearnet" &&
			m.status != nil &&
			m.status.publicIP != "" {
			uri = fmt.Sprintf(
				"lndconnect://%s:8080?macaroon=%s",
				m.status.publicIP,
				hexToBase64URL(mac))
			label = "Clearnet QR — " +
				m.status.publicIP + ":8080"
		} else if restOnion != "" && mac != "" {
			uri = fmt.Sprintf(
				"lndconnect://%s:8080?macaroon=%s",
				restOnion, hexToBase64URL(mac))
			label = "Tor QR — " +
				restOnion[:20] + "..."
		} else {
			return lipgloss.Place(
				m.width, m.height,
				lipgloss.Center, lipgloss.Center,
				theme.Warn.Render(
					"QR not available."))
		}
	}

	qr := renderQRCode(uri)
	var lines []string
	lines = append(lines,
		theme.Header.Render(label))
	lines = append(lines, "")
	if qr != "" {
		lines = append(lines, qr)
	}
	lines = append(lines, "")
	lines = append(lines, theme.Footer.Render(
		"backspace back • q quit"))
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Left, lines...))
}

func renderQRCode(data string) string {
	var buf bytes.Buffer
	config := qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     &buf,
		HalfBlocks: true,
		BlackChar:  qrterminal.BLACK_BLACK,
		WhiteChar:  qrterminal.WHITE_WHITE,
		QuietZone:  2,
	}
	qrterminal.GenerateWithConfig(data, config)
	return strings.TrimRight(buf.String(), "\n")
}

func hexToBase64URL(hexStr string) string {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
