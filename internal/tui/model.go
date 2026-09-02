package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"remiterm/internal/api"
	"remiterm/internal/cache"
)

type focusArea int

const (
	focusCompose focusArea = iota
	focusMessages
)

const (
	sendCooldown = 2 * time.Second
	// statusClearAfter is how long one-shot footer statuses ("sent",
	// "opened in browser") stay visible before auto-clear.
	statusClearAfter = 2 * time.Second
	// oldLoadBurstFree older pages may fire immediately when entering the
	// prefetch zone (still single-flight). Further auto loads wait for
	// oldLoadCooldown after the previous older fetch *completes*.
	oldLoadBurstFree = 1
	oldLoadCooldown  = 500 * time.Millisecond
	defaultLimit     = 50
	seedLimit        = 200
	composeHeight    = 3
	headerHeight     = 1
	footerHeight     = 1
	bannerHeight     = 1
)

// Model is the chat TUI.
type Model struct {
	client   *api.Client
	apiBase  string // for resolving relative media URLs
	poll     time.Duration
	store    *messageStore
	cacheDB  *cache.DB
	meHandle string

	viewport viewport.Model
	textarea textarea.Model
	focus    focusArea

	width  int
	height int
	ready  bool

	selected   int // index into store.order; -1 = none / follow live
	replyToID  string
	replyLabel string

	status     string // transient UX: "sent", "sending…", "opened in browser"
	statusGen  uint64 // bumps on each setTransientStatus; clear only if gen matches
	errMsg     string
	lastPoll   time.Time // last successful poll
	pollFailed bool      // most recent poll attempt failed
	loading    bool
	loadingOld bool
	showHelp   bool
	profile    profileModal
	preview    previewModal
	detail     detailView
	settings   settingsView
	prefs      Settings
	persist    PersistFunc

	// stickToBottom when true, auto-scroll on new messages.
	stickToBottom bool
	pendingNew    int

	// rate limit
	lastSendAt time.Time

	// older-history load gate (see tryLoadOlder / maybePrefetchOlder).
	lastOldFetchAt   time.Time // completion time of last older fetch
	oldLoadBurstUsed int       // consecutive older loads since leave prefetch zone
	oldSpinner       spinner.Model

	// rendered cache
	content   string
	itemSpans []lineSpan // viewport line range per list / detail row
}

// New constructs the TUI model.
func New(client *api.Client, poll time.Duration) Model {
	if poll <= 0 {
		poll = 5 * time.Second
	}
	ta := textarea.New()
	ta.Placeholder = "message…"
	ta.ShowLineNumbers = false
	ta.Prompt = "│ "
	ta.CharLimit = 8192
	ta.SetHeight(composeHeight)
	styles := ta.Styles()
	styles.Focused.Prompt = gutterStyle(true)
	styles.Blurred.Prompt = gutterStyle(false)
	ta.SetStyles(styles)
	ta.Focus()

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	// Mouse wheel needs program MouseMode, but cell-motion mouse tracking
	// steals clicks from OSC 8 hyperlinks in most terminals. Prefer clickable
	// media badges; scroll with keys (j/k, PgUp/Dn, g/G).

	apiBase := ""
	if client != nil {
		apiBase = client.BaseURL
	}
	sp := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(colorDim)),
	)
	return Model{
		client:        client,
		apiBase:       apiBase,
		poll:          poll,
		store:         newMessageStore(),
		viewport:      vp,
		textarea:      ta,
		focus:         focusCompose,
		selected:      -1,
		stickToBottom: true,
		oldSpinner:    sp,
		prefs:         defaultSettings(poll),
	}
}

// AttachCache seeds the in-memory store from a local SQLite cache.
// A nil db is a no-op (tests, remiterm demo).
func (m *Model) AttachCache(db *cache.DB) {
	m.cacheDB = db
	if db == nil || m.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := db.LatestMessages(ctx, seedLimit)
	if err != nil || len(msgs) == 0 {
		return
	}
	_ = m.store.mergeAPI(api.ListMessagesResponse{Messages: msgs, HasMore: true}, false)
}

func (m Model) persistMessages(msgs []api.Message) {
	if m.cacheDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, msg := range msgs {
		_ = m.cacheDB.SaveMessage(ctx, msg)
	}
}

func (m Model) persistProfile(p api.Profile) {
	if m.cacheDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = m.cacheDB.SaveProfile(ctx, p)
}

func (m Model) olderFromCache() []api.Message {
	if m.cacheDB == nil || m.store == nil || m.store.len() == 0 {
		return nil
	}
	oldest := m.store.idAt(0)
	if oldest == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := m.cacheDB.ListMessages(ctx, oldest, defaultLimit)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	return msgs
}

func (m Model) cachedProfile(handle string) (api.Profile, bool) {
	handle = strings.TrimSpace(handle)
	if m.cacheDB == nil || handle == "" || handle == "me" {
		return api.Profile{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, _, err := m.cacheDB.GetProfile(ctx, handle)
	if err != nil {
		return api.Profile{}, false
	}
	return p, true
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.fetchMe(),
		m.fetchLatest(false),
		m.tickPoll(),
	)
}

// --- messages ---

type meMsg struct {
	handle string
	err    error
}

type pollMsg struct {
	res   api.ListMessagesResponse
	err   error
	older bool
}

type sentMsg struct {
	msg api.Message
	err error
}

type tickMsg time.Time

// clearStatusMsg clears m.status only when statusGen still matches, so a newer
// transient status is not wiped by an older tick.
type clearStatusMsg struct {
	gen uint64
}

// setTransientStatus sets a one-shot footer status and schedules auto-clear.
// Empty s clears immediately without scheduling a tick. "sending…" should be
// assigned directly (in-flight), not via this helper.
func (m *Model) setTransientStatus(s string) tea.Cmd {
	m.status = s
	m.statusGen++
	gen := m.statusGen
	if s == "" {
		return nil
	}
	return tea.Tick(statusClearAfter, func(time.Time) tea.Msg {
		return clearStatusMsg{gen: gen}
	})
}

func (m Model) fetchMe() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		me, err := m.client.Me(ctx)
		if err != nil {
			return meMsg{err: err}
		}
		return meMsg{handle: me.Handle()}
	}
}

func (m Model) fetchLatest(older bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var (
			res api.ListMessagesResponse
			err error
		)
		if older {
			if m.store.cursor == "" && m.store.len() > 0 {
				// Use oldest id as before if cursor unknown.
				list := m.store.list()
				res, err = m.client.ListGlobalChat(ctx, defaultLimit, list[0].ID)
			} else {
				res, err = m.client.ListGlobalChat(ctx, defaultLimit, m.store.cursor)
			}
		} else {
			res, err = m.client.ListGlobalChat(ctx, defaultLimit, "")
		}
		return pollMsg{res: res, err: err, older: older}
	}
}

func (m Model) tickPoll() tea.Cmd {
	return tea.Tick(m.poll, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) send(text, replyTo string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		msg, err := m.client.PostGlobalChat(ctx, text, replyTo)
		return sentMsg{msg: msg, err: err}
	}
}

// --- update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.ready = true
		if m.preview.open && m.preview.img != nil {
			m.preview.art = m.renderPreviewArt()
		}
		m.redraw()
		return m, nil

	case meMsg:
		if msg.err != nil {
			m.errMsg = "whoami: " + msg.err.Error()
		} else {
			m.meHandle = msg.handle
		}
		return m, nil

	case tickMsg:
		cmds = append(cmds, m.tickPoll())
		if !m.loading {
			m.loading = true
			cmds = append(cmds, m.fetchLatest(false))
		}
		// Re-render so onlineLabel can flip to offline when stale.
		return m, tea.Batch(cmds...)

	case pollMsg:
		if msg.older {
			m.loadingOld = false
			// Cooldown clock starts on completion so latency does not eat the wait.
			m.lastOldFetchAt = time.Now()
		} else {
			m.loading = false
		}
		if msg.err != nil && msg.older {
			if cached := m.olderFromCache(); len(cached) > 0 {
				msg.err = nil
				msg.res = api.ListMessagesResponse{Messages: cached, HasMore: true}
			}
		}
		if msg.err != nil {
			// Live poll failures drive the connection indicator; older-page
			// errors stay as errMsg only so history backfill doesn't flip status.
			if !msg.older {
				m.pollFailed = true
			}
			m.errMsg = msg.err.Error()
			if msg.older {
				m.redraw()
				// Still allow paced retry while near the older edge.
				return m, m.maybePrefetchOlder()
			}
			return m, nil
		}
		if !msg.older {
			m.lastPoll = time.Now()
			m.pollFailed = false
		}
		m.errMsg = ""
		wasBottom := m.stickToBottom

		if msg.older {
			// Keep viewport position and selection identity across prepend.
			yBefore := m.viewport.YOffset()
			linesBefore := m.viewport.TotalLineCount()
			keepID := m.store.idAt(m.selected)
			_ = m.store.mergeAPI(msg.res, true)
			m.persistMessages(msg.res.Messages)
			if keepID != "" {
				if idx := m.store.indexOf(keepID); idx >= 0 {
					m.selected = idx
				}
			}
			m.redraw()
			anchorAfterPrepend(&m.viewport, yBefore, linesBefore)
			return m, m.maybePrefetchOlder()
		}

		keepID := m.store.idAt(m.selected)
		added := m.store.mergeLatest(msg.res)
		m.persistMessages(msg.res.Messages)
		if keepID != "" {
			if idx := m.store.indexOf(keepID); idx >= 0 {
				m.selected = idx
			} else if m.store.len() > 0 && m.selected >= m.store.len() {
				m.selected = m.store.len() - 1
			}
		}
		if !wasBottom && added > 0 {
			m.pendingNew += added
		}
		m.redraw()
		if wasBottom {
			m.viewport.GotoBottom()
		}
		return m, nil

	case spinner.TickMsg:
		if !m.loadingOld || msg.ID != m.oldSpinner.ID() {
			return m, nil
		}
		var cmd tea.Cmd
		m.oldSpinner, cmd = m.oldSpinner.Update(msg)
		m.redraw()
		return m, cmd

	case sentMsg:
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			// Drop in-flight "sending…" so it does not reappear after errMsg clears.
			_ = m.setTransientStatus("")
			// restore draft? already cleared — put text back only if we still have it.
			return m, nil
		}
		m.store.upsert(msg.msg)
		m.persistMessages([]api.Message{msg.msg})
		m.replyToID = ""
		m.replyLabel = ""
		m.lastSendAt = time.Now()
		cmd := m.setTransientStatus("sent")
		m.stickToBottom = true
		m.pendingNew = 0
		m.redraw()
		m.viewport.GotoBottom()
		return m, cmd

	case clearStatusMsg:
		if msg.gen == m.statusGen {
			m.status = ""
			m.redraw()
		}
		return m, nil

	case profileMsg:
		return m.applyProfileMsg(msg)

	case pokeMsg:
		return m.applyPokeMsg(msg)

	case statsMsg:
		return m.applyStatsMsg(msg)

	case previewMsg:
		return m.applyPreviewMsg(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Delegate to focused component.
	if m.focus == focusCompose {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
		m.syncStickFromViewport()
		if c := m.maybePrefetchOlder(); c != nil {
			cmds = append(cmds, c)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Preview overlay wins over profile, settings, detail, help, and chat.
	if m.preview.open {
		return m.handlePreviewKey(msg)
	}
	// Profile modal wins over settings, detail, help, and chat keybindings.
	if m.profile.open {
		return m.handleProfileKey(msg)
	}
	if m.settings.open {
		if m.showHelp {
			switch msg.String() {
			case "?", "ctrl+g", "esc", "q":
				m.showHelp = false
				return m, nil
			}
			return m, nil
		}
		return m.handleSettingsKey(msg)
	}
	// Message detail full view (not an overlay).
	if m.detail.open {
		if m.showHelp {
			switch msg.String() {
			case "?", "ctrl+g", "esc", "q":
				m.showHelp = false
				return m, nil
			}
			return m, nil
		}
		return m.handleDetailKey(msg)
	}

	if m.showHelp {
		switch msg.String() {
		case "?", "ctrl+g", "esc", "q":
			m.showHelp = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.focus == focusMessages {
			return m, tea.Quit
		}
		// in compose, q is typed
	case "ctrl+g":
		m.showHelp = true
		return m, nil
	case "?":
		if m.focus == focusMessages {
			m.showHelp = true
			return m, nil
		}
	case "ctrl+s":
		return m.openSettings()
	case "s":
		if m.focus == focusMessages {
			return m.openSettings()
		}
	case "p":
		if m.focus == focusMessages {
			return m.openSelectedProfile()
		}
	case "m":
		if m.focus == focusMessages {
			return m.openMeProfile()
		}
	case "tab":
		if m.focus == focusCompose {
			m.focus = focusMessages
			m.textarea.Blur()
			if m.selected < 0 && m.store.len() > 0 {
				m.selected = m.store.len() - 1
			}
		} else {
			m.focus = focusCompose
			m.textarea.Focus()
		}
		m.redraw()
		return m, nil
	case "esc":
		if m.replyToID != "" {
			m.replyToID = ""
			m.replyLabel = ""
			m.redraw()
			return m, nil
		}
		if m.focus == focusMessages {
			m.focus = focusCompose
			m.textarea.Focus()
			m.redraw()
		}
		return m, nil
	case "enter":
		if m.focus == focusCompose {
			return m.trySend()
		}
		// messages focus: open message detail
		return m.openSelectedDetail()
	case "ctrl+j":
		if m.focus == focusCompose {
			m.textarea.SetValue(m.textarea.Value() + "\n")
			return m, nil
		}
	case "r":
		// Reply (toggle) only in messages focus so compose can still type "r".
		if m.focus == focusMessages {
			return m.toggleReply()
		}
	case "R":
		if m.focus == focusMessages {
			return m, m.fetchLatest(false)
		}
	case "o":
		// Only in messages focus so compose can still type "o".
		if m.focus == focusMessages {
			return m.openSelectedMedia()
		}
	case "i":
		if m.focus == focusMessages {
			return m.openSelectedPreview()
		}
	case "g":
		if m.focus == focusMessages {
			if m.store.len() > 0 {
				m.selected = 0
			}
			m.stickToBottom = false
			m.redraw()
			m.viewport.GotoTop()
			m.ensureSelectedVisible()
			return m, m.tryLoadOlder(true)
		}
	case "G":
		if m.focus == focusMessages {
			if m.store.len() > 0 {
				m.selected = m.store.len() - 1
			}
			m.stickToBottom = true
			m.pendingNew = 0
			m.resetOldLoadBurst()
			m.redraw()
			m.viewport.GotoBottom()
			return m, nil
		}
	case "pgup", "ctrl+u":
		if m.focus == focusMessages {
			m.viewport.HalfPageUp()
			m.syncStickFromViewport()
			return m, m.maybePrefetchOlder()
		}
	case "pgdown", "ctrl+d":
		if m.focus == focusMessages {
			m.viewport.HalfPageDown()
			m.syncStickFromViewport()
			return m, m.maybePrefetchOlder()
		}
	case "up", "k":
		if m.focus == focusMessages {
			if m.selected > 0 {
				m.selected--
			} else if m.selected < 0 && m.store.len() > 0 {
				m.selected = m.store.len() - 1
			}
			m.stickToBottom = false
			m.redraw()
			m.ensureSelectedVisible()
			return m, m.maybePrefetchOlder()
		}
	case "down", "j":
		if m.focus == focusMessages {
			if m.selected >= 0 && m.selected < m.store.len()-1 {
				m.selected++
			}
			m.redraw()
			m.ensureSelectedVisible()
			if m.selected == m.store.len()-1 {
				m.stickToBottom = true
				m.pendingNew = 0
			}
			return m, m.maybePrefetchOlder()
		}
	}

	if m.focus == focusCompose {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	m.syncStickFromViewport()
	return m, tea.Batch(cmd, m.maybePrefetchOlder())
}

func (m Model) trySend() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.textarea.Value())
	if text == "" {
		return m, nil
	}
	if time.Since(m.lastSendAt) < sendCooldown {
		remain := sendCooldown - time.Since(m.lastSendAt)
		m.errMsg = fmt.Sprintf("rate limit: wait %.1fs", remain.Seconds())
		return m, nil
	}
	reply := m.replyToID
	m.textarea.Reset()
	m.status = "sending…"
	return m, m.send(text, reply)
}

// canLoadOlder reports whether an older-history fetch may start.
// force (g) bypasses burst cooldown but never overlaps an in-flight load.
func (m Model) canLoadOlder(force bool) bool {
	if m.store == nil || !m.store.hasMore || m.loadingOld {
		return false
	}
	if force {
		return true
	}
	// First oldLoadBurstFree pages are free; then pace with cooldown.
	if m.oldLoadBurstUsed < oldLoadBurstFree {
		return true
	}
	if !m.lastOldFetchAt.IsZero() && time.Since(m.lastOldFetchAt) < oldLoadCooldown {
		return false
	}
	return true
}

// maybePrefetchOlder starts an older-page fetch when the viewport is near the
// older edge (or content is still shorter than the runway). Compose focus and
// leave-zone reset the burst budget. Scroll-event storms collapse to single-
// flight + completion cooldown via tryLoadOlder.
func (m *Model) maybePrefetchOlder() tea.Cmd {
	if m.focus != focusMessages {
		return nil
	}
	if !needsOlderPrefetch(m.viewport.YOffset(), m.viewport.Height(), m.viewport.TotalLineCount()) {
		m.resetOldLoadBurst()
		return nil
	}
	return m.tryLoadOlder(false)
}

// tryLoadOlder starts an older-page fetch when the gate allows it.
// force=true is used by g (bypass post-burst cooldown).
func (m *Model) tryLoadOlder(force bool) tea.Cmd {
	if !m.canLoadOlder(force) {
		return nil
	}
	m.loadingOld = true
	if !force {
		m.oldLoadBurstUsed++
	}
	m.redraw()
	return tea.Batch(m.fetchLatest(true), m.oldSpinner.Tick)
}

// resetOldLoadBurst restores free consecutive older loads after leaving the
// prefetch zone.
func (m *Model) resetOldLoadBurst() {
	m.oldLoadBurstUsed = 0
}

// toggleReply starts a reply to the selected message, or cancels if that
// message is already the reply target.
func (m Model) toggleReply() (tea.Model, tea.Cmd) {
	if m.store.len() == 0 {
		return m, nil
	}
	idx := m.selected
	if idx < 0 {
		idx = m.store.len() - 1
	}
	list := m.store.list()
	if idx < 0 || idx >= len(list) {
		return m, nil
	}
	msg := list[idx]
	if m.replyToID == msg.ID {
		m.replyToID = ""
		m.replyLabel = ""
		m.redraw()
		return m, nil
	}
	return m.startReplyTo(msg)
}

func (m Model) startReplyTo(msg api.Message) (tea.Model, tea.Cmd) {
	m.replyToID = msg.ID
	m.replyLabel = replySnippetLabel(msg, m.prefs.AuthorLabel)
	m.focus = focusCompose
	m.textarea.Focus()
	m.redraw()
	return m, nil
}

// replyBannerLabel is the compose reply preview. Recomputed from the live
// author_label pref so cycling settings updates the banner immediately.
func (m Model) replyBannerLabel() string {
	if m.replyToID == "" {
		return ""
	}
	if msg, ok := m.store.get(m.replyToID); ok {
		return replySnippetLabel(msg, m.prefs.AuthorLabel)
	}
	return m.replyLabel
}

func (m Model) openSelectedMedia() (tea.Model, tea.Cmd) {
	if m.store.len() == 0 {
		m.errMsg = "no messages"
		return m, nil
	}
	idx := m.selected
	if idx < 0 {
		idx = m.store.len() - 1
	}
	list := m.store.list()
	if idx < 0 || idx >= len(list) {
		return m, nil
	}
	return m.openMediaFor(list[idx])
}

func (m *Model) syncStickFromViewport() {
	m.stickToBottom = m.viewport.AtBottom()
	if m.stickToBottom {
		m.pendingNew = 0
	}
}

func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.textarea.SetWidth(m.width - 2)
	m.textarea.SetHeight(composeHeight)

	banner := 0
	if m.replyToID != "" {
		banner = bannerHeight
	}
	// header + viewport + banner + textarea + footer (footer may wrap)
	vpH := m.height - headerHeight - banner - composeHeight - m.footerRows() - 1
	if vpH < 3 {
		vpH = 3
	}
	vpW := m.contentWidth()
	if vpW < 1 {
		vpW = 1
	}
	m.viewport.SetWidth(vpW)
	m.viewport.SetHeight(vpH)
}

// contentWidth is the viewport/message wrap width: terminal minus the
// always-on 1-cell focus gutter.
func (m Model) contentWidth() int {
	if m.width > 1 {
		return m.width - 1
	}
	return max(0, m.width)
}

func (m *Model) redraw() {
	m.layout()
	if m.detail.open {
		m.refreshDetailChain()
	}
	switch {
	case m.settings.open:
		m.content = m.renderSettings()
	case m.detail.open:
		m.content = m.renderDetail()
	default:
		m.content = m.renderMessages()
	}
	m.viewport.SetContent(m.content)
}

// refreshDetailChain rebuilds the chain from the store (polls may add children)
// and keeps selection on the same message id when possible.
func (m *Model) refreshDetailChain() {
	if !m.detail.open || m.detail.focusID == "" || m.store == nil {
		return
	}
	prevID := ""
	if it, ok := m.detailSelectedItem(); ok && it.Msg.ID != "" {
		prevID = it.Msg.ID
	}
	chain := m.store.replyChain(m.detail.focusID)
	if len(chain) == 0 {
		// Focus scrolled out of store — close quietly.
		m.detail = detailView{}
		return
	}
	m.detail.chain = chain
	// Prefer previous selection id; else focus item.
	sel := focusIndex(chain)
	if prevID != "" {
		for i, it := range chain {
			if it.Msg.ID == prevID {
				sel = i
				break
			}
		}
	}
	if sel < 0 {
		sel = 0
	}
	m.detail.selected = sel
}

func (m *Model) ensureSelectedVisible() {
	if m.selected < 0 || m.selected >= len(m.itemSpans) {
		return
	}
	revealSpan(&m.viewport, m.itemSpans[m.selected])
}

// --- view ---

func (m Model) View() tea.View {
	var content string
	switch {
	case m.showHelp && !m.profile.open && !m.preview.open:
		content = styleHelp.Render(helpText)
	case !m.ready:
		content = "loading…"
	default:
		content = m.renderMain()
		if m.profile.open {
			content = m.renderProfileOverlay(content)
		}
		if m.preview.open {
			content = m.renderPreviewOverlay(content)
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	// Do not enable MouseMode: application mouse tracking makes terminals
	// report clicks to the app instead of opening OSC 8 hyperlinks.
	return v
}

func (m Model) renderMain() string {
	header := m.renderHeader()

	var body strings.Builder
	body.WriteString(header)
	body.WriteString("\n")
	body.WriteString(withLeftGutter(m.viewport.View(), m.messagesRegionActive()))
	body.WriteString("\n")
	if m.replyToID != "" {
		banner := styleBanner.Render("↳ reply " + m.replyBannerLabel() + "  (esc cancel)")
		body.WriteString(withLeftGutter(banner, m.composeRegionActive()))
		body.WriteString("\n")
	}
	body.WriteString(m.textarea.View())
	body.WriteString("\n")
	body.WriteString(m.renderFooter())
	return body.String()
}

func (m *Model) renderMessages() string {
	list := m.store.list()
	if len(list) == 0 {
		m.itemSpans = nil
		return styleStatus.Render("no messages yet…")
	}
	var b strings.Builder
	prefix := 0
	// Stable tip only: loading state lives in the footer so prepend line
	// counts (and scroll anchor) are not disturbed mid-fetch.
	if m.store.hasMore {
		tip := styleStatus.Render("↑ scroll up for older")
		b.WriteString(tip)
		b.WriteString("\n")
		prefix = lipgloss.Height(tip)
	}
	blocks := make([]string, len(list))
	for i, msg := range list {
		blocks[i] = m.formatMessage(msg, i == m.selected && m.focus == focusMessages)
		b.WriteString(blocks[i])
		b.WriteString("\n")
	}
	m.itemSpans = lineSpans(blocks, prefix, 0)
	return b.String()
}

func (m Model) formatMessage(msg api.Message, selected bool) string {
	ts := formatTimestamp(messageTime(msg.CreatedAt), time.Now(), m.prefs.TimestampFormat)
	label := authorLabel(msg.Author, m.prefs.AuthorLabel)
	identity := authorHandle(msg.Author)

	hStyle := styleHandle
	if m.meHandle != "" && identity == m.meHandle {
		hStyle = styleHandleOwn
	}

	// Parent preview for replies (immediate parent only).
	var parentSnippet string
	var parentMissing bool
	if msg.ReplyToID != "" {
		if parent, ok := m.store.get(msg.ReplyToID); ok {
			parentSnippet = replySnippetLabel(parent, m.prefs.AuthorLabel)
		} else {
			parentMissing = true
		}
	}

	// Child body: text + media + reactions + edited.
	var body strings.Builder
	text := strings.ReplaceAll(msg.Text, "\n", "↵ ")
	body.WriteString(text)
	for _, med := range msg.Media {
		// Compact label; full path is useless noise. Hyperlink (OSC 8) makes it
		// clickable in supporting terminals; `o` always works with the absolute URL.
		href := absoluteURL(m.apiBase, med.URL)
		if href == "" {
			href = absoluteURL(m.apiBase, med.ThumbnailURL)
		}
		body.WriteString(" ")
		body.WriteString(mediaLabel(href, med.Kind))
	}
	if len(msg.Reactions) > 0 {
		var parts []string
		for _, r := range msg.Reactions {
			parts = append(parts, fmt.Sprintf("%s%d", r.Emoji, r.Count))
		}
		body.WriteString(styleTime.Render("  " + strings.Join(parts, " ")))
	}
	if isEdited(msg) {
		body.WriteString(styleTime.Render(" (edited)"))
	}
	bodyText := body.String()

	var line string
	compact := m.width > 0 && m.width < replyCompactWidth
	switch {
	case parentSnippet != "" && !compact:
		// Two-line (web-like): parent preview, then child without ↳@ prefix.
		var parentLine strings.Builder
		parentLine.WriteString(styleTime.Render(ts))
		parentLine.WriteString(" ")
		parentLine.WriteString(styleReply.Render("↳ " + parentSnippet))

		var childLine strings.Builder
		childLine.WriteString(styleTime.Render(ts))
		childLine.WriteString(" ")
		childLine.WriteString(hStyle.Render(label))
		childLine.WriteString(" ")
		childLine.WriteString(bodyText)
		line = parentLine.String() + "\n" + childLine.String()
	default:
		var b strings.Builder
		b.WriteString(styleTime.Render(ts))
		b.WriteString(" ")
		b.WriteString(hStyle.Render(label))
		b.WriteString(" ")
		if parentSnippet != "" {
			// Compact: ↳@handle: snippet · body on one line.
			b.WriteString(styleReply.Render("↳" + parentSnippet + " · "))
		} else if parentMissing {
			b.WriteString(styleReply.Render("↳ "))
		}
		b.WriteString(bodyText)
		line = b.String()
	}

	cw := m.contentWidth()
	if selected {
		return styleSelected.Width(max(0, cw)).Render(line)
	}
	// soft wrap via lipgloss
	if cw > 4 {
		return lipgloss.NewStyle().Width(cw).Render(line)
	}
	return line
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const helpText = `remiterm help

  Enter        send (compose) · open detail (messages)
  Ctrl+J       newline in draft
  Tab          toggle focus: compose ↔ messages
  j/k ↑/↓      select / scroll messages
  PgUp/PgDn    page scroll
  g / G        oldest / latest (select + scroll)
  r            reply to selected message (toggle)
  R            force refresh (messages focus)
  o            open media / URL (messages focus)
  i            preview media in-terminal (messages focus)
  p            profile of selected author (messages focus)
  m            own profile (messages focus)
  s            settings (messages focus)
  Ctrl+S       settings (compose)
  Esc          cancel reply / back to compose
  ?            toggle this help (messages)
  Ctrl+G       toggle this help (compose)
  q            quit (from messages focus)
  Ctrl+C       quit

Message detail (Enter from messages):
  j/k          move along reply chain
  Enter        re-root detail on selected chain item
  r            reply to selected → compose
  o            open media / URL
  i            preview media in-terminal
  p            profile of selected author
  Esc / q      back to messages (selects detail focus)

Settings (s from messages, Ctrl+S from compose):
  j/k          select setting
  Enter / h/l  cycle value
  Esc / q      back to messages

Profile modal:
  Esc / q      close
  P            poke (not yourself)
  o            open profile on remilia.net

Media preview (i):
  Esc / q      close
  o            open original in browser

Media shows as [image ↗] (clickable when the terminal supports it);
i previews a still as Unicode halfblocks (GIF first frame; video stays o);
o opens the URL in the default browser.

esc / ? / Ctrl+G / q to close
`
