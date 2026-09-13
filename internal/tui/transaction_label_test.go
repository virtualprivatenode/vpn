package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/virtualprivatenode/vpn/internal/lndrpc"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

type labelSaveClient struct {
	requests []lndrpc.TransactionLabelRequest
	result   lndrpc.TransactionLabelResult
}

func (c *labelSaveClient) SetTransactionLabel(request lndrpc.TransactionLabelRequest) lndrpc.TransactionLabelResult {
	c.requests = append(c.requests, request)
	return c.result
}

func labelModel() (Model, *OnChainHomeScreen, *labelSaveClient) {
	theme.Init(true)
	ctx := &ScreenContext{ContentFocused: true}
	oc := &OnChainContext{
		Utxos: []lndrpc.UTXO{{Txid: strings.Repeat("01", 32)}, {Txid: strings.Repeat("02", 32)}},
		OnChainTxs: []lndrpc.OnChainTx{
			{Txid: strings.Repeat("01", 32), Label: "first"},
			{Txid: strings.Repeat("02", 32), Label: "second"},
		},
	}
	client := &labelSaveClient{result: lndrpc.TransactionLabelResult{Submitted: true}}
	s := NewOnChainHomeScreen(ctx, oc)
	s.labels = client
	m := Model{nav: NewNavSidebar(), screenCtx: ctx, ocCtx: oc}
	m.sectionScreens[secOnChain] = s
	return m, s, client
}

func saveLabel(t *testing.T, s *OnChainHomeScreen, text string) tea.Cmd {
	t.Helper()
	s.labelInput.SetValue(text)
	s.HandleKey("tab", tea.KeyPressMsg{})
	_, cmd := s.HandleKey("enter", tea.KeyPressMsg{})
	if cmd == nil {
		t.Fatal("Save did not schedule a request")
	}
	return cmd
}

func TestLabelEditKeepsTargetAcrossUTXORefresh(t *testing.T) {
	for _, disappear := range []bool{false, true} {
		m, s, client := labelModel()
		target := s.ocCtx.Utxos[0].Txid
		s.openLabelPopup()
		utxos := []lndrpc.UTXO{s.ocCtx.Utxos[1], s.ocCtx.Utxos[0]}
		if disappear {
			utxos = nil
		}
		updated, _ := m.Update(utxoListMsg{utxos: utxos})
		m = updated.(Model)
		// The status and wallet observation are unavailable in this fixture.
		// Neither that nor an empty UTXO list may hide the active editor.
		if view := s.View(67, 28); !strings.Contains(view, "Transaction:") || !strings.Contains(view, "Save") {
			t.Fatal("refresh or unavailable status hid the editor")
		}
		command := saveLabel(t, s, " intended ")
		for _, key := range []tea.KeyPressMsg{
			{Code: tea.KeyEnter}, {Code: tea.KeyUp}, {Code: tea.KeyDown},
			{Code: tea.KeyTab}, {Code: tea.KeyRight}, {Code: 'x', Text: "x"},
		} {
			if _, duplicate := s.HandleKey(key.String(), key); duplicate != nil {
				t.Fatalf("pending edit accepted %q", key.String())
			}
		}
		s.HandleMsg(tea.PasteMsg{Content: "replacement"})
		if !s.labelEditing || !s.labelPending || s.labelInput.Value() != " intended " {
			t.Fatal("pending edit was closed, replaced or modified")
		}
		if _, nav := s.HandleKey("left", tea.KeyPressMsg{}); nav == nil {
			t.Fatal("pending edit cannot navigate away")
		}
		// Deliver through the model while Channels is visible.
		message := command()
		_, refresh := m.Update(message)
		if refresh == nil || s.labelPending || s.labelEditing || len(client.requests) != 1 ||
			client.requests[0].Txid.String() != target || client.requests[0].Label != " intended " ||
			s.ocCtx.OnChainTxs[0].Label != " intended " || s.ocCtx.OnChainTxs[1].Label != "second" {
			t.Fatalf("save changed target/text or lost hidden completion: %+v", client.requests)
		}
		if _, duplicate := m.Update(message); duplicate != nil {
			t.Fatal("duplicate completion scheduled another refresh")
		}
	}
}

func TestLabelEditFailureAndExplicitRetry(t *testing.T) {
	m, s, client := labelModel()
	s.openLabelPopup()
	client.result = lndrpc.TransactionLabelResult{Err: errors.New("not connected")}
	first := saveLabel(t, s, "draft")()
	if _, cmd := m.Update(first); cmd != nil || !s.labelEditing || s.labelPending ||
		!strings.Contains(s.labelErr, "not saved") || s.labelInput.Value() != "draft" {
		t.Fatal("pre-submission refusal lost draft or scheduled work")
	}
	client.result = lndrpc.TransactionLabelResult{Submitted: true, Err: errors.New("lost response")}
	retry := saveLabel(t, s, "draft")
	unknown := retry()
	if _, cmd := m.Update(unknown); cmd != nil || !s.labelEditing || s.labelPending ||
		!strings.Contains(s.labelErr, "not confirmed") || s.labelInput.Value() != "draft" || len(client.requests) != 2 {
		t.Fatal("unknown result lost its draft, uncertainty or explicit-retry boundary")
	}
	view := s.View(67, 28)
	if !strings.Contains(view, "not confirmed") || !strings.Contains(view, "retrying") {
		t.Fatal("outcome guidance is not rendered")
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 67 {
			t.Fatal("label result exceeds the content width")
		}
	}
	client.result = lndrpc.TransactionLabelResult{Submitted: true}
	current := saveLabel(t, s, "corrected label")
	_, refresh := m.Update(current())
	if refresh == nil || s.labelEditing || s.ocCtx.OnChainTxs[0].Label != "corrected label" || len(client.requests) != 3 {
		t.Fatal("explicit retry did not finish the intended edit")
	}
}

func TestLabelEditorValidatesBytesWithoutTruncatingText(t *testing.T) {
	m, s, client := labelModel()
	label := strings.Repeat("é", 250)
	s.ocCtx.OnChainTxs[0].Label = label
	s.openLabelPopup()
	if s.labelInput.Value() != label {
		t.Fatal("existing valid label was truncated")
	}
	s.labelInput.SetValue(label + "x")
	s.HandleKey("tab", tea.KeyPressMsg{})
	if _, cmd := s.HandleKey("enter", tea.KeyPressMsg{}); cmd != nil || s.labelErr == "" || s.labelInput.Value() != label+"x" {
		t.Fatal("overlong text was truncated or submitted instead of rejected")
	}
	command := saveLabel(t, s, label)
	_, refresh := m.Update(command())
	if refresh == nil || len(client.requests) != 1 || client.requests[0].Label != label {
		t.Fatal("valid corrected label did not reach the client intact")
	}
}

func TestLabelResultCannotReachReplacementScreenOrRevertHistory(t *testing.T) {
	m, s, _ := labelModel()
	oldRead := fetchOnChainTxCmd(nil, m.ocCtx)().(onChainTxMsg)
	oldRead.err = nil
	oldRead.txs = []lndrpc.OnChainTx{{Txid: s.ocCtx.Utxos[0].Txid, Label: "obsolete"}}
	s.openLabelPopup()
	result := saveLabel(t, s, "acknowledged")()
	_, refresh := m.Update(result)
	if refresh == nil {
		t.Fatal("successful label save did not refresh history")
	}
	m.Update(oldRead)
	if s.ocCtx.OnChainTxs[0].Label != "acknowledged" {
		t.Fatal("pre-save read undid the acknowledged label")
	}
	failed := refresh().(onChainTxMsg)
	m.Update(failed)
	if s.ocCtx.OnChainTxs[0].Label != "acknowledged" {
		t.Fatal("failed refresh erased the acknowledged label")
	}
	current := failed
	current.err = nil
	current.txs = []lndrpc.OnChainTx{{Txid: s.labelTxid, Label: "external change"}}
	m.Update(current)
	if s.ocCtx.OnChainTxs[0].Label != "external change" {
		t.Fatal("current daemon observation cannot supersede the local acknowledgement")
	}
	s.openLabelPopup()
	s.labelInput.SetValue("another draft")
	if _, cmd := m.Update(result); cmd != nil || !s.labelEditing || s.labelInput.Value() != "another draft" {
		t.Fatal("duplicate success closed a newer unsaved edit")
	}
	next := saveLabel(t, s, "next label")
	if _, cmd := m.Update(result); cmd != nil || !s.labelPending {
		t.Fatal("old success completed a newer submission")
	}
	if _, cmd := m.Update(next()); cmd == nil || s.labelPending || s.labelEditing {
		t.Fatal("current result was not delivered")
	}
	_, replacement, replacementClient := labelModel()
	m.sectionScreens[secOnChain] = replacement
	replacement.openLabelPopup()
	saveLabel(t, replacement, "replacement")
	if _, cmd := m.Update(result); cmd != nil || !replacement.labelPending || len(replacementClient.requests) != 0 {
		t.Fatal("old owner reached the replacement screen")
	}
	foreign := current
	foreign.owner = &OnChainContext{}
	foreign.txs = nil
	m.Update(foreign)
	if len(m.ocCtx.OnChainTxs) != 1 {
		t.Fatal("foreign history owner replaced current state")
	}
}
