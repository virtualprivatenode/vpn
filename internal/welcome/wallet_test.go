package welcome

import (
	"testing"
)

func TestCleanPayReq(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"lnbc100n1abc", "lnbc100n1abc"},
		{"[lnbc100n1abc]", "lnbc100n1abc"},
		{"lightning:lnbc100n1abc", "lnbc100n1abc"},
		{"LIGHTNING:lnbc100n1abc", "lnbc100n1abc"},
		{"\"lnbc100n1abc\"", "lnbc100n1abc"},
		{"  lnbc100n1abc  ", "lnbc100n1abc"},
	}
	for _, tt := range tests {
		got := cleanPayReq(tt.input)
		if got != tt.want {
			t.Errorf("cleanPayReq(%q): got %q, want %q",
				tt.input, got, tt.want)
		}
	}
}

func TestIsBolt11Char(t *testing.T) {
	valid := []rune{'a', 'z', '0', '9', 'A', 'Z'}
	for _, ch := range valid {
		if !isBolt11Char(ch) {
			t.Errorf("isBolt11Char(%c) should be true", ch)
		}
	}
	invalid := []rune{'[', ']', ' ', '\n', '!', '@', '#'}
	for _, ch := range invalid {
		if isBolt11Char(ch) {
			t.Errorf("isBolt11Char(%c) should be false", ch)
		}
	}
}

func TestParseRecvAmount(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"1000", 1000, false},
		{"1", 1, false},
		{"0", 0, true},
		{"", 0, true},
		{"abc", 0, true},
		{"1,000", 1000, false},
	}
	for _, tt := range tests {
		got, err := parseRecvAmount(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("parseRecvAmount(%q) should error", tt.input)
			continue
		}
		if !tt.wantErr && err != nil {
			t.Errorf("parseRecvAmount(%q) error: %v", tt.input, err)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseRecvAmount(%q): got %d, want %d",
				tt.input, got, tt.want)
		}
	}
}

func TestResetReceiveState(t *testing.T) {
	m := testModel()
	m.recvAmountInput = newRecvAmountInput()
	m.recvAmountInput.SetValue("1000")
	m.recvMemoInput = newRecvMemoInput()
	m.recvMemoInput.SetValue("test")
	m.recvPayReq = "lnbc..."
	m.recvPaymentHash = "abc"
	m.recvAmountSats = 1000
	m.recvSettled = true
	m.recvError = "some error"

	m.resetReceiveState()

	if m.recvAmountInput.Value() != "" {
		t.Error("recvAmountInput not reset")
	}
	if m.recvMemoInput.Value() != "" {
		t.Error("recvMemoInput not reset")
	}
	if m.recvPayReq != "" {
		t.Error("recvPayReq not reset")
	}
	if m.recvAmountSats != 0 {
		t.Error("recvAmountSats not reset")
	}
	if m.recvError != "" {
		t.Error("recvError not reset")
	}
}

func TestResetSendState(t *testing.T) {
	m := testModel()
	m.sendInput = newSendPayReqInput()
	m.sendInput.SetValue("lnbc...")
	m.sendDecodedAmt = 5000
	m.sendInFlight = true
	m.sendError = "error"
	m.sendPreimage = "preimage"
	m.sendFeeSats = 10

	m.resetSendState()

	if m.sendInput.Value() != "" {
		t.Error("sendInput not reset")
	}
	if m.sendDecodedAmt != 0 {
		t.Error("sendDecodedAmt not reset")
	}
	if m.sendInFlight {
		t.Error("sendInFlight not reset")
	}
	if m.sendError != "" {
		t.Error("sendError not reset")
	}
}

func TestIsWalletSubview(t *testing.T) {
	walletViews := []wSubview{
		svReceive, svReceiveWaiting, svReceivePaid,
		svReceiveExpired, svSend, svSendConfirm,
		svSendInFlight, svSendResult, svPaymentHistory,
		svPaymentDetail,
	}
	for _, sv := range walletViews {
		if !isWalletSubview(sv) {
			t.Errorf("isWalletSubview(%d) should be true", sv)
		}
	}
	if isWalletSubview(svNone) {
		t.Error("svNone should not be wallet")
	}
	if isWalletSubview(svChannelOpen) {
		t.Error("svChannelOpen should not be wallet")
	}
}

func TestFormatTimestamp(t *testing.T) {
	if formatTimestamp(0) != "—" {
		t.Error("zero should return dash")
	}
	// Non-zero should return something
	result := formatTimestamp(1700000000)
	if result == "" || result == "—" {
		t.Error("valid timestamp should return formatted string")
	}
}

func TestFormatTimestampFull(t *testing.T) {
	if formatTimestampFull(0) != "—" {
		t.Error("zero should return dash")
	}
	result := formatTimestampFull(1700000000)
	if result == "—" {
		t.Error("valid timestamp should format")
	}
}
