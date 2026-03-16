// internal/welcome/pairing.go

package welcome

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	qrterminal "github.com/mdp/qrterminal/v3"

	"github.com/ripsline/virtual-private-node/internal/paths"
	"github.com/ripsline/virtual-private-node/internal/theme"
)

func (m Model) viewPairing(bw int) string {
	cardW := bw - 2
	cardH := theme.BoxHeight

	zeusCard := m.zeusCard(cardW, cardH)
	return zeusCard
}

func (m Model) zeusCard(w, h int) string {
	var lines []string
	zeusEnabled := m.cfg.HasLND() && m.cfg.WalletExists()

	if zeusEnabled {
		restOnion := readOnion(paths.TorLNDRESTHostname)
		lines = append(lines, theme.Lightning.Render("⚡️ Zeus Wallet"))
		lines = append(lines, "")
		if m.cfg.P2PMode == "hybrid" {
			lines = append(lines, theme.Dim.Render("LND REST — Clearnet + Tor"))
		} else {
			lines = append(lines, theme.Dim.Render("LND REST over Tor"))
		}
		lines = append(lines, "")
		if restOnion != "" {
			lines = append(lines, theme.GreenDot.Render("●")+" ready")
		} else {
			lines = append(lines, theme.RedDot.Render("●")+" waiting for Tor")
		}
		lines = append(lines, "")
		lines = append(lines, theme.Action.Render("Select for setup ▸"))
	} else if m.cfg.HasLND() {
		lines = append(lines, theme.Grayed.Render("⚡️ Zeus Wallet"))
		lines = append(lines, "")
		lines = append(lines, theme.Grayed.Render("Create LND wallet first"))
	} else {
		lines = append(lines, theme.Grayed.Render("⚡️ Zeus Wallet"))
		lines = append(lines, "")
		lines = append(lines, theme.Grayed.Render("Install LND from Dashboard"))
	}

	border := theme.NormalBorder
	if zeusEnabled {
		border = theme.SelectedBorder
	}
	return border.Width(w).Padding(1, 2).Render(padLines(lines, h))
}

func (m Model) viewZeus() string {
	bw := min(m.width-4, theme.ContentWidth)
	var lines []string
	lines = append(lines, theme.Lightning.Render("⚡️ Zeus Wallet — LND REST"))
	lines = append(lines, "")

	restOnion := readOnion(paths.TorLNDRESTHostname)

	if m.cfg.P2PMode == "hybrid" {
		lines = append(lines, theme.Header.Render("  🛜 Clearnet Connection"))
		lines = append(lines, "")
		if m.status != nil && m.status.publicIP != "" {
			lines = append(lines, "  "+theme.Label.Render("Server: ")+
				theme.Mono.Render(m.status.publicIP))
			lines = append(lines, "  "+theme.Label.Render("REST Port: ")+
				theme.Mono.Render("8080"))
			lines = append(lines, "  "+theme.Dim.Render("  First connect: accept the certificate warning."))
			lines = append(lines, "  "+theme.Dim.Render("  The connection is encrypted."))
		} else {
			lines = append(lines, "  "+theme.Dim.Render("Public IP not available"))
		}
		lines = append(lines, "")
		lines = append(lines, theme.Header.Render("  🧅 Tor Connection"))
		lines = append(lines, "")
	}

	if restOnion == "" {
		lines = append(lines, "  "+theme.Warn.Render("Tor address not available yet."))
	} else {
		if m.cfg.P2PMode != "hybrid" {
			lines = append(lines, "  "+theme.Label.Render("Type: ")+
				theme.Mono.Render("LND (REST)"))
			lines = append(lines, "")
		}
		lines = append(lines, "  "+theme.Label.Render("Server: ")+
			theme.Mono.Render(restOnion))
		lines = append(lines, "  "+theme.Label.Render("REST Port: ")+
			theme.Mono.Render("8080"))
	}

	lines = append(lines, "")
	mac := readMacaroonHex(m.cfg)
	if mac != "" {
		preview := mac[:min(40, len(mac))] + "..."
		lines = append(lines, "  "+theme.Label.Render("Macaroon (Hex format):"))
		lines = append(lines, "  "+theme.Mono.Render(preview))
		lines = append(lines, "")
		if m.cfg.P2PMode == "hybrid" {
			lines = append(lines, "  "+theme.Action.Render("[m] full macaroon  [r] QR (Tor)  [c] QR (Clearnet)"))
		} else {
			lines = append(lines, "  "+theme.Action.Render("[m] full macaroon    [r] QR code"))
		}
	}

	lines = append(lines, "")
	lines = append(lines, theme.Dim.Render("1. download & verify Zeus"))
	lines = append(lines, theme.Dim.Render("2. Advanced Set-Up → LND (REST)"))
	lines = append(lines, theme.Dim.Render("3. Enter connection details or scan QR"))
	if m.cfg.P2PMode == "hybrid" {
		lines = append(lines, theme.Dim.Render("4. Clearnet is faster, Tor is more private"))
	}

	box := theme.Box.Width(bw).Padding(1, 2).Render(strings.Join(lines, "\n"))
	title := theme.Title.Width(bw).Align(lipgloss.Center).Render(" ⚡️ Zeus — Node Wallet ")
	var footer string
	if m.cfg.P2PMode == "hybrid" {
		footer = theme.Footer.Render("  m macaroon • r QR (Tor) • c QR (Clearnet) • backspace back • q quit  ")
	} else {
		footer = theme.Footer.Render("  m macaroon • r QR • backspace back • q quit  ")
	}
	full := lipgloss.JoinVertical(lipgloss.Center, "", title, "", box, "", footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, full)
}

func (m Model) viewQR() string {
	var uri string
	var label string

	if m.qrLabel != "" {
		// LndHub or other custom QR
		uri = m.urlTarget
		label = m.qrLabel
	} else {
		// Zeus QR (original behavior)
		restOnion := readOnion(paths.TorLNDRESTHostname)
		mac := readMacaroonHex(m.cfg)

		if m.qrMode == "clearnet" && m.status != nil && m.status.publicIP != "" {
			uri = fmt.Sprintf("lndconnect://%s:8080?macaroon=%s",
				m.status.publicIP, hexToBase64URL(mac))
			label = "Clearnet QR — " + m.status.publicIP + ":8080"
		} else if restOnion != "" && mac != "" {
			uri = fmt.Sprintf("lndconnect://%s:8080?macaroon=%s",
				restOnion, hexToBase64URL(mac))
			label = "Tor QR — " + restOnion[:20] + "..."
		} else {
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
				theme.Warn.Render("QR not available."))
		}
	}

	qr := renderQRCode(uri)
	var lines []string
	lines = append(lines, theme.Header.Render(label))
	lines = append(lines, "")
	if qr != "" {
		lines = append(lines, qr)
	}
	lines = append(lines, "")
	lines = append(lines, theme.Footer.Render("backspace back • q quit"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Left, lines...))
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
	result := buf.String()
	// Trim trailing newlines from qrterminal output
	return strings.TrimRight(result, "\n")
}

func hexToBase64URL(hexStr string) string {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
