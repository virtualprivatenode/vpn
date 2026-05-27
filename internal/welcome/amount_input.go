package welcome

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// AmountInput is a digits-only sats amount field with
// Sparrow-style live formatting:
//
//   - Only digits accepted. Non-digit keystrokes are
//     silently dropped; pasted non-digits are filtered out.
//   - Thousands separators inserted on every mutation:
//     "22542" renders as "22,542".
//   - Leading zeros collapse: "007" becomes "7".
//   - Backspace across a comma deletes the digit to the
//     left of the comma, not the comma itself ("1,234"
//     with cursor at end → "123", not "1,23").
//   - Cursor stays in a sensible place relative to the
//     digits the user was looking at when they typed.
//
// Internal model: we store the formatted (comma-separated)
// string in the wrapped textinput. All write operations
// pass through a single normalization step:
//
//  1. Capture raw digits + cursor position in digit-space.
//  2. Apply the mutation in digit-space.
//  3. Reformat to display-space with commas.
//  4. Translate cursor back to display-space and set.
//
// This one-pass model handles every case (typing mid-field,
// backspace, paste, programmatic writes) uniformly.
type AmountInput struct {
	ti textinput.Model
}

// NewAmountInput returns a focused sats amount input. All
// three original factories (on-chain send, Lightning
// receive, channel open) share this shape.
func NewAmountInput() AmountInput {
	ti := textinput.New()
	ti.Placeholder = "amount in sats"
	// CharLimit applies to the displayed (comma-inclusive)
	// string. 16 comfortably holds ~12 digits of sats.
	// Total BTC supply is ~2.1e15 sats (16 digits) — well
	// beyond any realistic input.
	ti.CharLimit = 16
	ti.SetWidth(20)
	ti.Prompt = "  "
	// Do NOT set ti.Validate. Validation happens in
	// Update() by filtering the keystroke. A Validate hook
	// would reject "22,542" because it contains a comma.
	applyInputStyles(&ti)
	return AmountInput{ti: ti}
}

// ── Delegating methods ─────────────────────────────────

func (a *AmountInput) Focus()        { a.ti.Focus() }
func (a *AmountInput) Blur()         { a.ti.Blur() }
func (a *AmountInput) Focused() bool { return a.ti.Focused() }
func (a *AmountInput) View() string  { return a.ti.View() }

// SetWidth sets the display width of the underlying
// textinput. Useful when a screen needs to adjust the
// width at render time (e.g., channel open's custom
// amount field sizes itself to fit the pane).
func (a *AmountInput) SetWidth(w int) { a.ti.SetWidth(w) }

// ── Value access ───────────────────────────────────────

// Sats returns the current value as an integer. 0 if the
// field is empty.
func (a *AmountInput) Sats() int64 {
	return parseSats(a.ti.Value())
}

// SetSats writes an integer value with comma formatting.
// A zero or negative value clears the field.
func (a *AmountInput) SetSats(n int64) {
	if n <= 0 {
		a.ti.SetValue("")
		return
	}
	a.ti.SetValue(formatWithCommas(n))
	a.ti.SetCursor(len(a.ti.Value()))
}

// Clear resets the value to empty.
func (a *AmountInput) Clear() {
	a.ti.SetValue("")
}

// Empty reports whether the field contains no digits.
func (a *AmountInput) Empty() bool {
	return strings.TrimSpace(a.ti.Value()) == ""
}

// CursorAtEnd reports whether the cursor is at or past
// the end of the current value. Used by screens that
// implement right-arrow-escapes-to-button behavior.
func (a *AmountInput) CursorAtEnd() bool {
	return a.ti.Position() >= len(a.ti.Value())
}

// ── Update: the single mutation path ───────────────────

// Update feeds a message into the amount input and
// applies the Sparrow-style behaviors. Returns any
// tea.Cmd emitted by the underlying textinput for
// non-mutating messages (blink, focus events).
func (a *AmountInput) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.PasteMsg:
		return a.handlePaste(m)
	case tea.KeyPressMsg:
		return a.handleKey(m)
	}
	// Non-key, non-paste messages pass through (cursor
	// blink, etc.) — they don't mutate the value so no
	// reformatting is needed.
	var cmd tea.Cmd
	a.ti, cmd = a.ti.Update(msg)
	return cmd
}

// handleKey handles a single key press. Named navigation
// keys pass through; digit keys mutate and reformat;
// non-digit single-rune keys are dropped.
func (a *AmountInput) handleKey(
	msg tea.KeyPressMsg,
) tea.Cmd {
	key := msg.String()

	if key == "backspace" {
		a.mutate(deleteLeft)
		return nil
	}

	// Non-printable keys (left, right, home, end, etc.)
	// pass through unchanged. They don't alter the value,
	// only the cursor, which is already in display-space
	// in the wrapped textinput.
	if !isSingleRune(key) {
		var cmd tea.Cmd
		a.ti, cmd = a.ti.Update(msg)
		return cmd
	}

	// Single-rune key: must be a digit.
	r := []rune(key)[0]
	if r < '0' || r > '9' {
		return nil
	}

	a.mutate(insertDigit(byte(r)))
	return nil
}

// handlePaste strips non-digits from the paste payload
// and inserts the result at the cursor.
func (a *AmountInput) handlePaste(
	msg tea.PasteMsg,
) tea.Cmd {
	pasted := filterDigits(msg.Content)
	if pasted == "" {
		return nil
	}
	a.mutate(insertString(pasted))
	return nil
}

// ── The mutation path ──────────────────────────────────

// mutate applies a digit-space mutation to the current
// value, reformats, and restores the cursor to the
// equivalent position in display-space.
//
// Every write path (typing a digit, backspace, paste)
// reduces to the same three steps. The mutation itself
// is the only thing that differs.
func (a *AmountInput) mutate(mut mutation) {
	val := a.ti.Value()
	cursor := a.ti.Position()

	// 1. Capture state in digit-space.
	raw, digitPos := toDigitSpace(val, cursor)

	// 2. Apply the mutation in digit-space.
	newRaw, newDigitPos := mut(raw, digitPos)

	// Collapse leading zeros ONLY when the string has a
	// non-zero digit. Until then, show what the user
	// typed ("0", "00", "000"…). The moment a real digit
	// arrives, collapse all leading zeros. Matches
	// Sparrow's behavior: every keystroke is visible
	// feedback, but the field never settles on a
	// misleading "0001234" representation of a value.
	trimmed := newRaw
	if hasNonZeroDigit(newRaw) {
		var collapsed int
		trimmed, collapsed = trimLeadingZeros(newRaw)
		if newDigitPos >= collapsed {
			newDigitPos -= collapsed
		} else {
			newDigitPos = 0
		}
	}

	// Enforce CharLimit on the would-be formatted string
	// (paste can push us past). Truncate raw from the
	// right until it fits.
	if a.ti.CharLimit > 0 {
		for len(formatDigitsWithCommas(trimmed)) >
			a.ti.CharLimit && len(trimmed) > 0 {
			trimmed = trimmed[:len(trimmed)-1]
			if newDigitPos > len(trimmed) {
				newDigitPos = len(trimmed)
			}
		}
	}

	// 3. Reformat and set cursor.
	// Skip comma formatting for all-zero strings
	// ("0000" stays "0000", not "0,000").
	formatted := trimmed
	if hasNonZeroDigit(trimmed) {
		formatted = formatDigitsWithCommas(trimmed)
	}
	a.ti.SetValue(formatted)
	a.ti.SetCursor(
		fromDigitSpace(formatted, newDigitPos))
}

// mutation is a pure transformation in digit-space:
// given a raw-digit string and a cursor position within
// it, return the new string and new cursor position.
type mutation func(
	raw string, pos int,
) (newRaw string, newPos int)

func insertDigit(d byte) mutation {
	return func(raw string, pos int) (string, int) {
		return raw[:pos] + string(d) + raw[pos:], pos + 1
	}
}

func insertString(s string) mutation {
	return func(raw string, pos int) (string, int) {
		return raw[:pos] + s + raw[pos:], pos + len(s)
	}
}

func deleteLeft(raw string, pos int) (string, int) {
	if pos == 0 {
		return raw, 0
	}
	return raw[:pos-1] + raw[pos:], pos - 1
}

// ── Space translation ──────────────────────────────────

// toDigitSpace converts a display-space (comma-formatted)
// value and cursor position to a raw-digit string and
// digit-space cursor position. The digit-space position
// counts digits strictly before the cursor, ignoring
// commas.
//
// A cursor immediately after a comma maps to the same
// digit-space position as one immediately before the
// comma — both sit "between the two digits the comma
// separates." This is what makes backspace-across-comma
// natural: from the user's perspective the comma is
// invisible.
func toDigitSpace(
	val string, displayPos int,
) (string, int) {
	var raw strings.Builder
	digitPos := 0
	for i, c := range val {
		if c == ',' {
			continue
		}
		if i < displayPos {
			digitPos++
		}
		raw.WriteRune(c)
	}
	return raw.String(), digitPos
}

// fromDigitSpace converts a digit-space position back to
// a display-space position in a formatted string. A
// digit-space position of N lands the cursor immediately
// after the Nth digit.
func fromDigitSpace(
	formatted string, digitPos int,
) int {
	if digitPos <= 0 {
		return 0
	}
	count := 0
	for i, c := range formatted {
		if c == ',' {
			continue
		}
		count++
		if count == digitPos {
			return i + 1
		}
	}
	return len(formatted)
}

// ── Formatting primitives ──────────────────────────────

// parseSats parses a comma-formatted sats string.
// Replaces parseSendAmount / parseRecvAmount /
// parseCustomAmount in helpers.go.
func parseSats(val string) int64 {
	val = strings.ReplaceAll(val, ",", "")
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	var n int64
	for _, c := range val {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

// formatWithCommas formats an int64 with thousands
// separators: 22542 → "22,542". Negative values are
// prefixed with '-' but this field never accepts them
// in practice.
func formatWithCommas(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 24)
	digits := 0
	for n > 0 {
		if digits > 0 && digits%3 == 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, byte('0'+n%10))
		n /= 10
		digits++
	}
	// Reverse.
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}

// formatDigitsWithCommas formats a raw-digit string
// (already stripped of commas and sign).
func formatDigitsWithCommas(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	head := len(digits) % 3
	if head == 0 {
		head = 3
	}
	var b strings.Builder
	b.Grow(len(digits) + len(digits)/3)
	b.WriteString(digits[:head])
	for i := head; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// trimLeadingZeros returns the string with leading zeros
// removed, and the count of zeros removed.
func trimLeadingZeros(s string) (string, int) {
	i := 0
	for i < len(s) && s[i] == '0' {
		i++
	}
	return s[i:], i
}

// hasNonZeroDigit reports whether s contains at least one
// digit other than '0'. Used to defer leading-zero
// collapse until a real value starts forming.
func hasNonZeroDigit(s string) bool {
	for _, c := range s {
		if c >= '1' && c <= '9' {
			return true
		}
	}
	return false
}

// filterDigits returns s with all non-digit runes removed.
func filterDigits(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// isSingleRune reports whether s contains exactly one
// rune. Used to distinguish typed characters from named
// keys in tea.KeyPressMsg.String() output.
func isSingleRune(s string) bool {
	if s == "" {
		return false
	}
	n := 0
	for range s {
		n++
		if n > 1 {
			return false
		}
	}
	return n == 1
}
