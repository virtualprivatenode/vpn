// internal/bitcoin/client_test.go

package bitcoin

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestBlockchainInfoParsing(t *testing.T) {
	raw := `{
        "blocks": 850000,
        "headers": 850100,
        "verificationprogress": 0.9998,
        "initialblockdownload": false,
        "chain": "main"
    }`

	var resp blockchainInfoResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Blocks != 850000 {
		t.Errorf("Blocks: got %d, want 850000", resp.Blocks)
	}
	if resp.Headers != 850100 {
		t.Errorf("Headers: got %d, want 850100", resp.Headers)
	}
	if resp.VerificationProgress != 0.9998 {
		t.Errorf("VerificationProgress: got %f, want 0.9998", resp.VerificationProgress)
	}
	if resp.InitialBlockDownload {
		t.Error("InitialBlockDownload: expected false")
	}
}

func TestBlockchainInfoObservation(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		blocks int
		synced bool
	}{
		{`{"blocks":100000,"headers":850000,"verificationprogress":0.1234,"initialblockdownload":true,"unknown_future_field":42}`, 100000, false},
		{`{"blocks":0,"headers":0,"initialblockdownload":false}`, 0, true},
	} {
		info, err := readBlockchainInfo(func(result any) error { return json.Unmarshal([]byte(tc.raw), result) })
		if err != nil || info.Blocks != tc.blocks || info.Synced != tc.synced {
			t.Fatalf("chain observation: %+v %v", info, err)
		}
	}
	failed := errors.New("RPC unavailable")
	info, err := readBlockchainInfo(func(any) error { return failed })
	if !errors.Is(err, failed) || info.Synced {
		t.Fatal("failed read became a synced empty chain")
	}
}

func TestBlockchainInfoExtraFields(t *testing.T) {
	// bitcoin-cli returns many fields we don't use.
	// Verify they don't break parsing.
	raw := `{
        "chain": "main",
        "blocks": 850000,
        "headers": 850000,
        "bestblockhash": "0000000000000000000abc123",
        "difficulty": 86388558925171.02,
        "time": 1700000000,
        "mediantime": 1699999000,
        "verificationprogress": 0.999999,
        "initialblockdownload": false,
        "chainwork": "00000000000000000000000000000000",
        "size_on_disk": 600000000000,
        "pruned": true,
        "pruneheight": 800000,
        "warnings": ""
    }`

	var resp blockchainInfoResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal should ignore extra fields: %v", err)
	}

	if resp.Blocks != 850000 {
		t.Errorf("Blocks: got %d, want 850000", resp.Blocks)
	}
}

func TestSignetIdentityFieldsParsing(t *testing.T) {
	raw := `{"chain":"signet","signet_challenge":"5121","blocks":1}`
	var resp blockchainInfoResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Chain != "signet" || resp.SignetChallenge != "5121" {
		t.Fatalf("identity fields: %+v", resp)
	}
}

func TestFormatProgress(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0.0, "0.00%"},
		{0.5, "50.00%"},
		{0.9999, "99.99%"},
		{1.0, "100.00%"},
		{0.1234, "12.34%"},
	}
	for _, tt := range tests {
		got := FormatProgress(tt.input)
		if got != tt.want {
			t.Errorf("FormatProgress(%f): got %q, want %q",
				tt.input, got, tt.want)
		}
	}
}
