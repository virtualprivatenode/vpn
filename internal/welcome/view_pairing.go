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
		var lines []string
		lines = append(lines,
			theme.Lightning.Render(" ⚡ Zeus Wallet"))
		lines = append(lines, "")
		if m.cfg.HasLND() {
			lines = append(lines, " "+theme.Dim.Render(
				"Create LND wallet first"))
		} else {
			lines = append(lines, " "+theme.Dim.Render(
				"Install LND first"))
		}
		return strings.Join(lines, "\n")
	}

	if m.status == nil || !m.status.lndResponding {
		var lines []string
		lines = append(lines,
			theme.Lightning.Render(" ⚡ Zeus Wallet"))
		lines = append(lines, "")
		lines = append(lines, " "+theme.Dim.Render(
			"Waiting for LND..."))
		return strings.Join(lines, "\n")
	}

	isFocused := m.contentFocused && !m.tabFocused

	var lines []string
	lines = append(lines,
		theme.Lightning.Render(" ⚡ Zeus — LND REST"))
	lines = append(lines, "")

	restOnion := readOnion(paths.TorLNDRESTHostname)

	if m.cfg.P2PMode == "hybrid" {
		lines = append(lines,
			" "+theme.Header.Render("🛜 Clearnet"))
		if m.status != nil && m.status.publicIP != "" {
			lines = append(lines,
				" "+theme.Label.Render("Server: ")+
					theme.Mono.Render(
						m.status.publicIP))
			lines = append(lines,
				" "+theme.Label.Render("Port:   ")+
					theme.Mono.Render("8080"))
		}
		lines = append(lines, "")
		lines = append(lines,
			" "+theme.Header.Render("🧅 Tor"))
	}

	if restOnion == "" {
		lines = append(lines,
			" "+theme.Warn.Render("Tor not available"))
	} else {
		server := restOnion
		if len(server) > w-14 {
			server = server[:w-17] + "..."
		}
		lines = append(lines,
			" "+theme.Label.Render("Server: ")+
				theme.Mono.Render(server))
		lines = append(lines,
			" "+theme.Label.Render("Port:   ")+
				theme.Mono.Render("8080"))
	}

	mac := readMacaroonHex(m.cfg)
	if mac != "" {
		lines = append(lines, "")
		preview := mac[:min(24, len(mac))] + "..."
		lines = append(lines,
			" "+theme.Label.Render("Macaroon: ")+
				theme.Mono.Render(preview))
	}

	lines = append(lines, "")

	// Buttons
	maxBtn := 1
	if m.cfg.P2PMode == "hybrid" {
		maxBtn = 2
	}

	btnLabels := []string{"QR (Tor)", "Macaroon"}
	if m.cfg.P2PMode == "hybrid" {
		btnLabels = append(btnLabels, "QR (Clearnet)")
	}

	var btnParts []string
	for i, label := range btnLabels {
		isActive := isFocused &&
			m.pairingButtonIdx == i
		if isActive {
			btnParts = append(btnParts,
				"▸ "+theme.BtnFocused.Render(label))
		} else {
			btnParts = append(btnParts,
				"  "+theme.BtnNormal.Render(label))
		}
	}
	_ = maxBtn
	lines = append(lines,
		" "+strings.Join(btnParts, "  "))

	return strings.Join(lines, "\n")
}

// handlePairingEnter processes enter on pairing buttons.
func (m Model) handlePairingEnter() (tea.Model, tea.Cmd) {
	switch m.pairingButtonIdx {
	case 0: // QR (Tor)
		restOnion := readOnion(paths.TorLNDRESTHostname)
		mac := readMacaroonHex(m.cfg)
		if restOnion != "" && mac != "" {
			m.urlTarget = fmt.Sprintf(
				"lndconnect://%s:8080?macaroon=%s",
				restOnion, hexToBase64URL(mac))
			m.qrLabel = "Tor QR — " +
				restOnion[:min(20, len(restOnion))] +
				"..."
			m.qrMode = "tor"
			m.urlReturnTo = svWalletPairing
			m.subview = svQR
		}
	case 1: // Macaroon
		return m, showMacaroonCmd(m.cfg)
	case 2: // QR (Clearnet)
		if m.cfg.P2PMode == "hybrid" &&
			m.status != nil &&
			m.status.publicIP != "" {
			mac := readMacaroonHex(m.cfg)
			if mac != "" {
				m.urlTarget = fmt.Sprintf(
					"lndconnect://%s:8080?macaroon=%s",
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
		restOnion := readOnion(paths.TorLNDRESTHostname)
		mac := readMacaroonHex(m.cfg)

		if m.qrMode == "clearnet" && m.status != nil &&
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
