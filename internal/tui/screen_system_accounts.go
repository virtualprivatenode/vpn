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
	focusZone, btnIdx, confirmIdx         int
	technical                             bool
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
			visible := s.visibleAccounts()
			if s.cursor < len(visible) {
				selected = visible[s.cursor].Ref()
			}
			s.accounts, s.cursor = msg.accounts, 0
			for i, a := range s.visibleAccounts() {
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

func (s *AccountsScreen) visibleAccounts() []accountaccess.Account {
	var visible []accountaccess.Account
	for _, a := range s.accounts {
		if a.KeyDiscoverySupported() {
			visible = append(visible, a)
		}
	}
	return visible
}

func (s *AccountsScreen) buttons() []string {
	if s.technical {
		return []string{"Back", "Refresh"}
	}
	if s.account != nil {
		return []string{"Back", "Technical details", "Refresh"}
	}
	return []string{"Refresh"}
}

func (s *AccountsScreen) listLen() int {
	if s.technical {
		return 0
	}
	if s.account == nil {
		return len(s.visibleAccounts())
	}
	if s.detail != nil {
		return len(s.detail.Source.Keys)
	}
	return 0
}

func (s *AccountsScreen) back() tea.Cmd {
	s.scroll, s.btnIdx, s.focusZone = 0, 0, sshZoneButtons
	if s.technical {
		s.technical = false
		return nil
	}
	if s.account == nil {
		return emitFocusParent
	}
	s.request++ // Retire any outstanding detail read.
	s.account, s.detail, s.err, s.loading = nil, nil, nil, false
	return s.refresh()
}

func (s *AccountsScreen) HandleKey(k string, _ tea.KeyPressMsg) (Screen, tea.Cmd) {
	if k == "ctrl+c" {
		return s, tea.Quit
	}
	if s.working {
		return s, nil
	}
	if s.resultReady {
		switch k {
		case "pgdown":
			s.scroll += 8
		case "pgup":
			s.scroll = max(0, s.scroll-8)
		case "enter":
			s.resultReady, s.review, s.detail = false, nil, nil
			s.focusZone, s.btnIdx, s.scroll = sshZoneButtons, 0, 0
			return s, s.refresh()
		}
		return s, nil
	}
	if s.review != nil {
		switch k {
		case "pgdown":
			s.scroll += 8
		case "pgup":
			s.scroll = max(0, s.scroll-8)
		case "left":
			if s.confirmIdx == 0 {
				return s, emitFocusSidebar
			}
			s.confirmIdx = 0
		case "right":
			s.confirmIdx = 1
		case "up", "shift+tab":
			return s, emitFocusTabBar
		case "esc", "backspace":
			s.review, s.scroll = nil, 0
		case "enter":
			if s.confirmIdx == 0 {
				s.review, s.scroll = nil, 0
				return s, nil
			}
			s.working = true
			s.attempt++
			attempt, review, access := s.attempt, *s.review, s.ctx.accountAccess()
			return s, func() tea.Msg { return accountImportMsg{owner: s, attempt: attempt, err: access.Import(review)} }
		}
		return s, nil
	}
	switch k {
	case "left":
		if s.focusZone == sshZoneButtons && s.btnIdx > 0 {
			s.btnIdx--
			return s, nil
		}
		return s, emitFocusSidebar
	case "right":
		if s.focusZone == sshZoneButtons {
			s.btnIdx = min(len(s.buttons())-1, s.btnIdx+1)
		}
	case "up", "shift+tab":
		s.scroll = 0
		if s.focusZone == sshZoneButtons {
			return s, emitFocusTabBar
		}
		cursor := &s.cursor
		if s.account != nil {
			cursor = &s.keyCursor
		}
		if k == "shift+tab" || *cursor == 0 {
			s.focusZone, s.btnIdx = sshZoneButtons, 0
		} else {
			*cursor--
		}
	case "down", "tab":
		s.scroll = 0
		if s.listLen() == 0 {
			return s, nil
		}
		if s.focusZone == sshZoneButtons {
			s.focusZone = sshZoneKeys
			return s, nil
		}
		if s.account == nil {
			s.cursor = min(s.listLen()-1, s.cursor+1)
		} else {
			s.keyCursor = min(s.listLen()-1, s.keyCursor+1)
		}
	case "backspace", "esc":
		return s, s.back()
	case "r":
		return s, s.refresh()
	case "pgdown":
		s.scroll += 8
	case "pgup":
		s.scroll = max(0, s.scroll-8)
	case "enter":
		if s.focusZone == sshZoneButtons {
			if s.account != nil && s.btnIdx == 0 {
				return s, s.back()
			}
			if s.account != nil && !s.technical && s.btnIdx == 1 {
				s.technical, s.btnIdx, s.scroll = true, 0, 0
				return s, nil
			}
			return s, s.refresh()
		}
		if s.loading || s.err != nil {
			return s, nil
		}
		if s.account == nil {
			visible := s.visibleAccounts()
			if s.cursor < len(visible) {
				a := visible[s.cursor]
				s.account, s.scroll, s.focusZone, s.btnIdx = &a, 0, sshZoneButtons, 0
				return s, s.refresh()
			}
		} else if s.detail != nil && s.detail.Source.Problem == "" && s.detail.OwnerKeysProblem == "" &&
			s.detail.Account.Name != "vpn" && s.keyCursor < len(s.detail.Source.Keys) {
			key := s.detail.Source.Keys[s.keyCursor]
			if !s.detail.Authorized[key.Fingerprint] {
				s.review = &app.AccountKeyImport{Account: s.detail.Account, Key: key}
				s.scroll, s.confirmIdx = 0, 0
			}
		}
	}
	return s, nil
}

// Keep navigation fixed while account lists, key lists and optional diagnostics scroll.
func (s *AccountsScreen) renderBody(header, body *paneBuilder, w, h, cursor int) string {
	head := header.render()
	bodyText := body.render()
	lines := strings.Count(bodyText, "\n") + 1
	vpH := max(1, h-strings.Count(head, "\n")-1)
	s.scroll = min(s.scroll, max(0, lines-vpH))
	if s.scroll > 0 {
		cursor += s.scroll + vpH - 1
	}
	cursor = min(lines-1, cursor)
	return head + "\n" + renderViewport(bodyText, w, vpH, cursor, lines, true)
}

func accountText(p *paneBuilder, style lipgloss.Style, text string) {
	wrapped := style.Width(max(1, p.w-2)).Render(theme.PlainText(text))
	for _, line := range strings.Split(wrapped, "\n") {
		p.line(" " + line)
	}
}

func (s *AccountsScreen) View(w, h int) string {
	w, h = max(1, w), max(1, h)
	if s.review != nil || s.resultReady {
		return s.viewImport(w, h)
	}
	header, body := newPane(w), newPane(w)
	title := "Accounts"
	if s.account != nil {
		title = theme.PlainText(s.account.Name)
	}
	if s.technical {
		title += " · Technical details"
	}
	header.title(theme.Header, title).buttons(s.buttons(), s.btnIdx, s.ctx.ContentFocused && s.focusZone == sshZoneButtons)
	cursor := 0
	switch {
	case s.err != nil:
		body.warn("Observation unavailable")
		accountText(body, theme.Warning, s.err.Error())
		body.blank().dim("Choose Refresh to try again.")
	case s.account == nil:
		body.valueWrap("Use an SSH key from another account to connect as vpn.").blank()
		if s.loading {
			body.dim("Refreshing accounts...").blank()
		}
		visible := s.visibleAccounts()
		if s.loaded && len(visible) == 0 {
			body.dim("No local accounts to show.")
		}
		for i, a := range visible {
			selected := s.focusZone == sshZoneKeys && s.cursor == i
			if selected {
				cursor = len(body.lines)
			}
			style, marker := theme.Value, " "
			if selected && s.ctx.ContentFocused {
				style, marker = theme.NavActive, "▸"
			}
			body.line(marker + " " + style.Render(theme.PlainText(a.Name)))
			role := "Login account"
			switch {
			case a.Name == "vpn":
				role = "Your node account"
			case a.Name == "root":
				role = "Protected system account"
			}
			accountText(body, theme.Dim, "  "+role)
			body.blank()
		}
	case s.detail == nil:
		body.dim("Reading account access...")
	case s.technical:
		s.renderTechnical(body)
	default:
		d := s.detail
		if s.loading {
			body.dim("Refreshing key status...").blank()
		}
		switch {
		case !d.Account.KeyDiscoverySupported():
			body.valueWrap("Key import is not offered for this service or non-login account.")
		case d.Source.Problem != "":
			body.warn("Key discovery unavailable").valueWrap("The keys could not be read safely. Open Technical details for the reason.")
		case d.OwnerKeysProblem != "":
			body.warn("vpn key status unavailable").valueWrap("We could not check which keys vpn already has. Refresh before importing.")
		case d.Account.Name == "vpn":
			body.valueWrap("These are your node's configured SSH keys. Use System → SSH Keys to add or remove keys.")
		case len(d.Source.Keys) == 0:
			body.valueWrap("No supported keys found in this account's standard SSH key file.")
		default:
			available := 0
			for _, k := range d.Source.Keys {
				if !d.Authorized[k.Fingerprint] {
					available++
				}
			}
			if available == 0 {
				body.success("No import needed").valueWrap("All keys shown below are already configured for vpn.")
			} else {
				body.valueWrap("Select a key to review before adding it to vpn. Use a key only if you recognize its owner.")
			}
		}
		if d.Source.Excluded > 0 {
			body.blank().valueWrap(fmt.Sprintf("%d restricted or unsupported key entries cannot be imported.", d.Source.Excluded))
		}
		body.blank()
		for i, k := range d.Source.Keys {
			selected := s.focusZone == sshZoneKeys && s.keyCursor == i
			if selected {
				cursor = len(body.lines)
			}
			label := theme.PlainText(k.Comment)
			if label == "" {
				label = "SSH key"
			}
			style, marker := theme.Value, " "
			if selected && s.ctx.ContentFocused {
				style, marker = theme.NavActive, "▸"
			}
			accountText(body, style, marker+" "+label)
			body.monoWrap(k.Fingerprint)
			switch {
			case d.Source.Problem != "" || d.OwnerKeysProblem != "":
				body.warn("  Status unavailable")
			case d.Account.Name == "vpn" || d.Authorized[k.Fingerprint]:
				body.success("  Already configured for vpn")
			default:
				body.dim("  Enter to review import")
			}
			body.blank()
		}
	}
	return s.renderBody(header, body, w, h, cursor)
}

func (s *AccountsScreen) renderTechnical(p *paneBuilder) {
	d := s.detail
	p.field("UID: ", fmt.Sprint(d.Account.UID)).field("GID: ", fmt.Sprint(d.Account.GID))
	accountText(p, theme.Value, "Home: "+d.Account.Home)
	accountText(p, theme.Value, "Shell: "+d.Account.Shell)
	p.blank().labelLine("Local groups")
	if d.GroupsProblem != "" {
		accountText(p, theme.Warning, d.GroupsProblem)
	} else {
		accountText(p, theme.Value, strings.Join(d.Groups, ", "))
	}
	p.blank().labelLine("Public-key source")
	accountText(p, theme.Mono, d.Source.Path)
	if d.Source.Problem != "" {
		accountText(p, theme.Warning, d.Source.Problem)
	}
	if d.OwnerKeysProblem != "" {
		accountText(p, theme.Warning, d.OwnerKeysProblem)
	}
	p.blank().valueWrap("Only the standard authorized_keys file is inspected. Custom paths, SSH certificates and provider-managed access are not inventoried. A configured key does not prove a working login.")
	p.blank().labelLine("Sudo rules").valueWrap("Group membership alone does not prove sudo access.")
	if d.SudoProblem != "" {
		accountText(p, theme.Warning, d.SudoProblem)
	}
	for _, line := range strings.Split(d.SudoListing, "\n") {
		accountText(p, theme.Mono, line)
	}
}

func (s *AccountsScreen) viewImport(w, h int) string {
	p := newPane(w)
	labels, active, focused := []string{"Go Back", "Import Key"}, s.confirmIdx, s.ctx.ContentFocused
	if s.resultReady {
		labels, active = []string{"Return"}, 0
		if s.resultErr != nil {
			p.title(theme.Warning, "Import not confirmed")
			accountText(p, theme.Warning, s.resultErr.Error())
			p.blank().valueWrap("Refresh the SSH Keys list before retrying.")
		} else {
			p.title(theme.Success, "Key imported")
			p.valueWrap("Keep this session open and test a new SSH connection as vpn using this key.")
		}
	} else {
		p.title(theme.Header, "Import key into vpn")
		p.field("From: ", theme.PlainText(s.review.Account.Name))
		accountText(p, theme.Value, theme.PlainText(s.review.Key.Comment))
		p.labelLine("Fingerprint:").monoWrap(s.review.Key.Fingerprint).blank()
		p.warnWrapWords("The holder of this key will gain vpn SSH and node access, including wallet access.")
		p.blank().valueWrap("The source account and key are kept. Sudo policy stays the same.")
		if s.working {
			labels, active, focused = []string{"Importing..."}, 0, false
		}
	}
	// Keep the action reachable on short terminals while the explanation scrolls.
	buttons := renderButtons(labels, active, focused, w)
	vpH := max(1, h-strings.Count(buttons, "\n")-2)
	text := p.render()
	lines := strings.Count(text, "\n") + 1
	s.scroll = min(s.scroll, max(0, lines-vpH))
	content := renderViewport(text, w, vpH, min(lines-1, s.scroll+vpH-1), lines, true)
	return content + "\n\n" + buttons
}

func (s *AccountsScreen) HelpBindings() []key.Binding {
	if s.working {
		return waitingBindings()
	}
	if s.resultReady {
		return []key.Binding{bind("enter", "return", "enter"), bind("pgup/pgdn", "scroll", "pgup", "pgdown"), kQuit}
	}
	if s.review != nil || s.technical {
		return append(tabButtonBindings(s.ctx.HasTabs), bind("pgup/pgdn", "scroll", "pgup", "pgdown"))
	}
	list := "accounts"
	if s.account != nil {
		list = "keys"
	}
	if s.focusZone == sshZoneButtons {
		return manageButtonBindings(list, s.btnIdx, s.ctx.HasTabs)
	}
	return []key.Binding{bind("↑↓", list, "up", "down"), bind("enter", "open", "enter"),
		bind("⇧tab", "buttons", "shift+tab"), bind("r", "refresh", "r"), bind("pgup/pgdn", "scroll", "pgup", "pgdown"), kBack, kSidebar, kQuit}
}
