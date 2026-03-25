package welcome

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/ripsline/virtual-private-node/internal/theme"
)

// ── Validators ───────────────────────────────────────────

func validateDigits(s string) error {
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return errInvalidChar{}
		}
	}
	return nil
}

func isBolt11Char(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= '0' && ch <= '9') ||
		(ch >= 'A' && ch <= 'Z')
}

func validateBolt11(s string) error {
	for _, ch := range s {
		if !isBolt11Char(ch) {
			return errInvalidChar{}
		}
	}
	return nil
}

func validateHex(s string) error {
	for _, ch := range s {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return errInvalidChar{}
		}
	}
	return nil
}

func validateHostChars(s string) error {
	for _, ch := range s {
		if ch < 32 || ch > 126 {
			return errInvalidChar{}
		}
	}
	return nil
}

func validateHubName(s string) error {
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == ' ' || ch == '-') {
			return errInvalidChar{}
		}
	}
	return nil
}

func validateSyncthingID(s string) error {
	for _, ch := range s {
		upper := ch
		if ch >= 'a' && ch <= 'z' {
			upper = ch - 32
		}
		if !((upper >= 'A' && upper <= 'Z') || (upper >= '0' && upper <= '9') || upper == '-') {
			return errInvalidChar{}
		}
	}
	return nil
}

func validatePrintableASCII(s string) error {
	for _, ch := range s {
		if ch < 32 || ch >= 127 {
			return errInvalidChar{}
		}
	}
	return nil
}

// errInvalidChar is a sentinel returned by validators to reject input.
type errInvalidChar struct{}

func (errInvalidChar) Error() string { return "invalid character" }

// ── Input styles ─────────────────────────────────────────

func inputPromptStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
}

func inputTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
}

func inputPlaceholderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
}

func inputCursorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
}

func applyInputStyles(ti *textinput.Model) {
	s := textinput.DefaultStyles(true) // true = dark background
	ti.SetStyles(s)
}

// ── Factory functions ────────────────────────────────────

func newSendPayReqInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "lnbc..."
	ti.CharLimit = 1500
	ti.SetWidth(58)
	ti.Validate = validateBolt11
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newRecvAmountInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "amount in sats"
	ti.CharLimit = 10
	ti.SetWidth(20)
	ti.Validate = validateDigits
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newRecvMemoInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "optional memo"
	ti.CharLimit = 100
	ti.SetWidth(40)
	ti.Validate = validatePrintableASCII
	ti.Prompt = "  "
	applyInputStyles(&ti)
	// Starts blurred — amount field is focused first
	ti.Blur()
	return ti
}

func newChanPubkeyInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "pubkey (66 hex chars)"
	ti.CharLimit = 66
	ti.SetWidth(66)
	ti.Validate = validateHex
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newChanHostInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "host:port"
	ti.CharLimit = 80
	ti.SetWidth(50)
	ti.Validate = validateHostChars
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Blur()
	return ti
}

func newChanAmountInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "amount in sats"
	ti.CharLimit = 10
	ti.SetWidth(20)
	ti.Validate = validateDigits
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newHubNameInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "account name"
	ti.CharLimit = 30
	ti.SetWidth(30)
	ti.Validate = validateHubName
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newSyncthingIDInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "XXXXXXX-XXXXXXX-XXXXXXX-XXXXXXX-XXXXXXX-XXXXXXX-XXXXXXX-XXXXXXX"
	ti.CharLimit = 63
	ti.SetWidth(63)
	ti.Validate = validateSyncthingID
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newOnChainAddrInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "bc1q..."
	ti.CharLimit = 90
	ti.SetWidth(50)
	ti.Validate = validateOnChainAddr
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newOnChainAmtInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "amount in sats"
	ti.CharLimit = 16
	ti.SetWidth(20)
	ti.Validate = validateDigits
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func newCustomFeeInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "sat/vB"
	ti.CharLimit = 6
	ti.SetWidth(10)
	ti.Validate = validateDigits
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.Focus()
	return ti
}

func validateOnChainAddr(s string) error {
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9')) {
			return errInvalidChar{}
		}
	}
	return nil
}

func newUtxoLabelInput(current string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Enter label..."
	ti.CharLimit = 64
	ti.SetWidth(40)
	ti.Prompt = "  "
	applyInputStyles(&ti)
	ti.SetValue(current)
	ti.Focus()
	return ti
}

// ── Helpers ──────────────────────────────────────────────

// syncthingIDValue returns the uppercased value of the syncthing input.
func syncthingIDValue(ti textinput.Model) string {
	return strings.ToUpper(ti.Value())
}

// Unused import guard for theme — used by styles referencing the dark theme.
var _ = theme.Value
