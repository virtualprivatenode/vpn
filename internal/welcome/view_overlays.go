package welcome

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"charm.land/lipgloss/v2"
	qrterminal "github.com/mdp/qrterminal/v3"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Fullscreen overlays ────────────────────────────────
// These are Model-owned views that take over the entire
// screen (not section content). Triggered by setting
// m.subview to svQR or svFullURL.

func (m Model) viewQR() string {
	uri := m.urlTarget
	label := m.qrLabel

	if uri == "" {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			theme.Warn.Render(
				"QR not available."))
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
		"enter back"))
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Left, lines...))
}

func (m Model) viewFullURL() string {
	title := theme.Header.Render(
		"Full URL — Copy and paste into Tor Browser")
	hint := theme.Dim.Render(
		"Select and copy. Press enter to go back.")
	content := lipgloss.JoinVertical(lipgloss.Left,
		"", title, "", hint, "", m.urlTarget, "")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center, content)
}

// ── QR and encoding utilities ──────────────────────────

// renderQRCode generates a terminal-friendly QR code from
// the given data using qrterminal's halfblocks mode.
//
// All four halfblock character fields are set explicitly,
// even though qrterminal v3.2.1's GenerateWithConfig fills
// in defaults for any unset ones. Being explicit means the
// code no longer depends on that library default-filling
// behavior — if the library is ever downgraded to a version
// that doesn't fill defaults, or replaced by a fork that
// doesn't, the config still produces correct output.
func renderQRCode(data string) string {
	var buf bytes.Buffer
	config := qrterminal.Config{
		Level:          qrterminal.L,
		Writer:         &buf,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK_BLACK,
		WhiteChar:      qrterminal.WHITE_WHITE,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		QuietZone:      2,
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
