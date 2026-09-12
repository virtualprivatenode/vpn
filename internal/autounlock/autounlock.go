// Package autounlock defines wallet-password values and verified transition results.
package autounlock

import (
	"errors"
	"strings"
)

// Outcome is the classified end state of an auto-unlock change.
// Helper transport success means the root side finished observing and
// classifying the node; the outcome says whether the requested setting was
// applied, safely rolled back, or needs operator repair. Keeping these states
// structured prevents the TUI from parsing privileged error text.
type Outcome string

const (
	Enabled              Outcome = "enabled"
	Disabled             Outcome = "disabled"
	VerificationFailed   Outcome = "verification_failed"
	VerificationTimedOut Outcome = "verification_timed_out"
	StillEnabled         Outcome = "still_enabled"
	RepairRequired       Outcome = "repair_required"
)

// Result carries no credential material. FailedStep is a bounded,
// operator-facing stage name for logs and the repair-required screen; Detail
// is used only for a safely classified retry state.
type Result struct {
	Outcome    Outcome `json:"outcome"`
	FailedStep string  `json:"failed_step,omitempty"`
	Detail     string  `json:"detail,omitempty"`
}

// Password contains an exact wallet secret. Its zero value is invalid.
// This is independent of the operator login-password policy.
type Password struct{ text string }

// NewPassword validates the existing single-line, 512-byte helper contract.
// LND remains responsible for checking the password against the wallet.
func NewPassword(text string) (Password, error) {
	if text == "" {
		return Password{}, errors.New("wallet password is empty")
	}
	if len(text) > 512 {
		return Password{}, errors.New("wallet password is implausibly long")
	}
	if strings.ContainsAny(text, "\r\n") {
		return Password{}, errors.New("wallet password has a line break")
	}
	return Password{text: text}, nil
}

// Text exposes the secret only for local IPC and protected password publication.
func (p Password) Text() string   { return p.text }
func (Password) String() string   { return "[redacted]" }
func (Password) GoString() string { return "[redacted]" }
