package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/virtualprivatenode/vpn/internal/accountaccess"
	"github.com/virtualprivatenode/vpn/internal/app"
	"github.com/virtualprivatenode/vpn/internal/theme"
)

type accountAccess interface {
	List() ([]accountaccess.Account, error)
	Inspect(accountaccess.Ref) (app.AccountDetails, error)
	Import(app.AccountKeyImport) error
	Close()
}

func (c *ScreenContext) accountAccess() accountAccess {
	if c.AccountAccess == nil {
		c.AccountAccess = app.NewAccountAccess(c.sshAccess())
	}
	return c.AccountAccess
}

type accountsMsg struct {
	owner    *AccountsScreen
	request  uint64
	accounts []accountaccess.Account
	err      error
}
type accountDetailMsg struct {
	owner   *AccountsScreen
	request uint64
	detail  app.AccountDetails
	err     error
}
type accountImportMsg struct {
	owner   *AccountsScreen
	attempt uint64
	err     error
}

// AccountsScreen keeps a review fixed until canceled or submitted. Reads and
// mutations return to this exact screen even when another section is visible.
type AccountsScreen struct {
	ctx                                   *ScreenContext
	accounts                              []accountaccess.Account
	cursor                                int
	account                               *accountaccess.Account
	detail                                *app.AccountDetails
	keyCursor                             int
	request, attempt                      uint64
	loading, loaded, working, resultReady bool
	err                                   error
	review                                *app.AccountKeyImport
	resultErr                             error
	scroll                                int
}

func NewAccountsScreen(ctx *ScreenContext) *AccountsScreen { return &AccountsScreen{ctx: ctx} }
func (s *AccountsScreen) Init() tea.Cmd                    { return s.refresh() }

func openAccountsCmd(ctx *ScreenContext) tea.Cmd {
	return func() tea.Msg {
		return openTabMsg{Kind: tabAccounts, Label: "Accounts", Screen: NewAccountsScreen(ctx)}
	}
}

func (s *AccountsScreen) refresh() tea.Cmd {
	if s.loading || s.working || s.review != nil || s.resultReady {
		return nil
	}
	s.request++
	s.loading, s.err = true, nil
	request, access := s.request, s.ctx.accountAccess()
	if s.account != nil {
		ref := s.account.Ref()
		return func() tea.Msg {
			detail, err := access.Inspect(ref)
			return accountDetailMsg{owner: s, request: request, detail: detail, err: err}
		}
	}
	return func() tea.Msg {
		accounts, err := access.List()
		return accountsMsg{owner: s, request: request, accounts: accounts, err: err}
	}
}

func (s *AccountsScreen) HandleMsg(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tabActivatedMsg:
		return s, s.refresh()
	case accountsMsg:
		if msg.owner != s || msg.request != s.request || s.account != nil || !s.loading {
			return s, nil
		}
		s.loading, s.loaded, s.err = false, true, msg.err
		if msg.err == nil {
			var selected accountaccess.Ref
			if s.cursor < len(s.accounts) {
				selected = s.accounts[s.cursor].Ref()
			}
			s.accounts, s.cursor = msg.accounts, 0
			for i, a := range s.accounts {
				if a.Ref() == selected {
					s.cursor = i
					break
				}
			}
		}
	case accountDetailMsg:
		if msg.owner != s || msg.request != s.request || !s.loading || s.review != nil || s.working || s.account == nil {
			return s, nil
		}
		s.loading, s.err = false, msg.err
		if msg.err == nil {
			if msg.detail.Account.Ref() != s.account.Ref() {
				s.err = errors.New("account observation changed; refresh before continuing")
				return s, nil
			}
			fingerprint := ""
			if s.detail != nil && s.keyCursor < len(s.detail.Source.Keys) {
				fingerprint = s.detail.Source.Keys[s.keyCursor].Fingerprint
			}
			s.detail, s.keyCursor = &msg.detail, 0
			for i, k := range s.detail.Source.Keys {
				if k.Fingerprint == fingerprint {
					s.keyCursor = i
					break
				}
			}
		}
	case accountImportMsg:
		if msg.owner != s || msg.attempt != s.attempt || !s.working {
			return s, nil
		}
		s.working, s.resultReady, s.resultErr, s.scroll = false, true, msg.err, 0
		return s, refreshSSHKeysCmd
	}
	return s, nil
}

func (s *AccountsScreen) HandleKey(k string, _ tea.KeyPressMsg) (Screen, tea.Cmd) {
	if k == "ctrl+c" {
		return s, tea.Quit
	}
	if s.working {
		return s, nil
	}
	if s.resultReady {
		if k == "enter" {
			s.resultReady, s.review, s.detail = false, nil, nil
			return s, s.refresh()
		}
		return s, nil
	}
	if s.review != nil {
		switch k {
		case "pgdown":
			s.scroll += 8
		case "pgup":
			s.scroll -= 8
		case "n", "esc", "backspace":
			s.review = nil
			s.scroll = 0
		case "y":
			s.working = true
			s.attempt++
			attempt, review, access := s.attempt, *s.review, s.ctx.accountAccess()
			return s, func() tea.Msg { return accountImportMsg{owner: s, attempt: attempt, err: access.Import(review)} }
		}
		return s, nil
	}
	switch k {
	case "left":
		return s, emitFocusSidebar
	case "shift+tab":
		return s, emitFocusTabBar
	case "backspace", "esc":
		if s.account == nil {
			return s, emitFocusParent
		}
		s.request++ // Retire any outstanding detail read.
		s.account, s.detail, s.err, s.loading, s.scroll = nil, nil, nil, false, 0
		return s, s.refresh()
	case "r":
		return s, s.refresh()
	case "pgdown":
		s.scroll += 8
	case "pgup":
		s.scroll -= 8
	case "up":
		s.scroll = 0
		if s.account == nil {
			if s.cursor == 0 {
				return s, emitFocusTabBar
			}
			s.cursor--
		} else {
			s.keyCursor = max(0, s.keyCursor-1)
		}
	case "down", "tab":
		s.scroll = 0
		if s.account == nil {
			s.cursor = min(max(0, len(s.accounts)-1), s.cursor+1)
		} else if s.detail != nil {
			s.keyCursor = min(max(0, len(s.detail.Source.Keys)-1), s.keyCursor+1)
		}
	case "enter":
		if s.loading || s.err != nil {
			return s, nil
		}
		if s.account == nil {
			if s.cursor < len(s.accounts) {
				a := s.accounts[s.cursor]
				s.account, s.scroll = &a, 0
				return s, s.refresh()
			}
		} else if s.detail != nil && s.detail.Source.Problem == "" && s.detail.OwnerKeysProblem == "" &&
			s.detail.Account.Name != "vpn" && s.keyCursor < len(s.detail.Source.Keys) {
			key := s.detail.Source.Keys[s.keyCursor]
			if !s.detail.Authorized[key.Fingerprint] {
				s.review = &app.AccountKeyImport{Account: s.detail.Account, Key: key}
				s.scroll = 0
			}
		}
	}
	return s, nil
}

func (s *AccountsScreen) View(w, h int) string {
	w, h = max(1, w), max(1, h)
	var lines []string
	add := func(text string) {
		wrapped := lipgloss.NewStyle().Width(max(1, w-2)).Render(theme.PlainText(text))
		for _, line := range strings.Split(wrapped, "\n") {
			lines = append(lines, " "+line)
		}
	}
	add("Local accounts")
	lines = append(lines, "")
	focusLine := 0
	switch {
	case s.resultReady:
		if s.resultErr != nil {
			add("Import was not confirmed: " + s.resultErr.Error())
			add("Refresh the owner key list before retrying.")
		} else {
			add("Public key imported into vpn.")
			add("Keep this session open and test a new SSH login as vpn with that key.")
		}
		add("Press Enter to return to the account.")
	case s.review != nil:
		add("Import public key into vpn?")
		add("Source: " + s.review.Account.Name)
		add("Fingerprint: " + s.review.Key.Fingerprint)
		add("Comment: " + s.review.Key.Comment)
		add("The holder of its private key will gain vpn SSH and node access, including wallet access. Import does not change sudo policy.")
		add("The source account and its key file are retained.")
		if s.working {
			add("Rechecking source and importing...")
		} else {
			add("Import this key? [y/n]")
		}
	case s.err != nil:
		add("Observation unavailable: " + s.err.Error())
		add("Press r to retry.")
	case s.account == nil:
		add("Select an account to inspect its groups, sudo rules and supported public keys.")
		add("Local accounts only. External identity services are not inventoried.")
		if s.loading {
			add("Refreshing accounts...")
		}
		if s.loaded && len(s.accounts) == 0 {
			add("No local accounts found.")
		}
		for i, a := range s.accounts {
			marker := "  "
			if i == s.cursor {
				marker = "> "
				focusLine = len(lines)
			}
			add(fmt.Sprintf("%s%s  UID %d  %s", marker, a.Name, a.UID, a.Shell))
		}
	case s.detail == nil:
		add("Reading account access...")
	default:
		d := s.detail
		add(fmt.Sprintf("%s  UID %d  GID %d", d.Account.Name, d.Account.UID, d.Account.GID))
		add("Home: " + d.Account.Home)
		add("Shell: " + d.Account.Shell)
		if d.GroupsProblem != "" {
			add("Groups unavailable: " + d.GroupsProblem)
		} else {
			add("Local groups: " + strings.Join(d.Groups, ", "))
		}
		if s.loading {
			add("Refreshing access...")
		}
		add("Public-key file: " + d.Source.Path)
		if d.Source.Problem != "" {
			add("Key discovery incomplete: " + d.Source.Problem)
		} else if len(d.Source.Keys) == 0 {
			add("No supported keys in this standard file.")
		}
		if d.Source.Excluded > 0 {
			add(fmt.Sprintf("%d restricted or unsupported key entries excluded.", d.Source.Excluded))
		}
		if d.OwnerKeysProblem != "" {
			add("vpn key status unavailable: " + d.OwnerKeysProblem)
		}
		for i, k := range d.Source.Keys {
			marker, state := "  ", "Enter to review import"
			if d.Authorized[k.Fingerprint] {
				state = "Already configured for vpn"
			}
			if d.OwnerKeysProblem != "" {
				state = "vpn key status unknown"
			}
			if d.Account.Name == "vpn" {
				state = "Owner key; manage in SSH Keys"
			}
			if i == s.keyCursor {
				marker = "> "
				focusLine = len(lines)
			}
			add(marker + k.Fingerprint)
			add("  " + k.Type + "  " + k.Comment)
			add("  " + state)
		}
		add("Configured keys do not prove a working login. Custom key paths, SSH certificates and provider-managed access are not inventoried.")
		lines = append(lines, "")
		add("Sudo observation (group membership alone is not proof):")
		if d.SudoProblem != "" {
			add(d.SudoProblem)
		}
		for _, line := range strings.Split(d.SudoListing, "\n") {
			add(line)
		}
	}
	if s.scroll != 0 {
		focusLine = max(0, min(len(lines)-1, focusLine+s.scroll))
	}
	return renderViewport(strings.Join(lines, "\n"), w, h, focusLine, len(lines), true)
}

func (s *AccountsScreen) HelpBindings() []key.Binding {
	if s.working {
		return waitingBindings()
	}
	if s.resultReady {
		return []key.Binding{bind("enter", "return", "enter"), kQuit}
	}
	if s.review != nil {
		return confirmDialogBindings()
	}
	return []key.Binding{bind("↑↓", "select", "up", "down"), bind("enter", "open", "enter"),
		bind("r", "refresh", "r"), bind("pgup/pgdn", "scroll", "pgup", "pgdown"), kBack, kSidebar, kQuit}
}
