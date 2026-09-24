package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/osto/securecli/internal/auth"
)

type model struct {
	service         *auth.Service
	input           textinput.Model
	mode            screenMode
	user            auth.User
	expiresAt       time.Time
	history         []string
	historyIndex    int
	status          string
	quitting        bool
	sessionTimeout  time.Duration
	pendingUsername string
	pendingPassword string
	width           int
}

type screenMode string

const (
	modeCommand          screenMode = "command"
	modeAuthenticated    screenMode = "authenticated"
	modeRegisterUsername screenMode = "register-username"
	modeRegisterPassword screenMode = "register-password"
	modeLoginUsername    screenMode = "login-username"
	modeLoginPassword    screenMode = "login-password"
	modeLoginTOTP        screenMode = "login-totp"
	maxHistoryEntries               = 100
	operationTimeout                = 10 * time.Second
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB86B"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A9099"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8BE9A8"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7B72"))
)

func New(service *auth.Service, sessionTimeout time.Duration) tea.Model {
	input := textinput.New()
	input.Prompt = "> "
	input.CharLimit = 256
	input.Focus()
	return &model{service: service, input: input, mode: modeCommand, historyIndex: -1, status: "Type help to see available commands.", sessionTimeout: sessionTimeout, width: 80}
}

func (m *model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.expireSession()
	m.applyMessage(msg)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			return m, m.submit()
		case "up":
			m.previousHistory()
			return m, nil
		case "down":
			m.nextHistory()
			return m, nil
		case "tab":
			m.complete()
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.input.Width = msg.Width - 4
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *model) submit() tea.Cmd {
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		return nil
	}
	if m.mode == modeRegisterUsername {
		m.pendingUsername = value
		m.mode = modeRegisterPassword
		m.setInput("Password: ", textinput.EchoPassword)
		m.status = "Choose a password with at least 10 characters."
		return nil
	}
	if m.mode == modeRegisterPassword {
		return m.register(m.pendingUsername, value)
	}
	if m.mode == modeLoginUsername {
		m.pendingUsername = value
		m.mode = modeLoginPassword
		m.setInput("Password: ", textinput.EchoPassword)
		m.status = "Enter your password."
		return nil
	}
	if m.mode == modeLoginPassword {
		m.pendingPassword = value
		return m.login(m.pendingUsername, value, "")
	}
	if m.mode == modeLoginTOTP {
		return m.login(m.pendingUsername, m.pendingPassword, value)
	}

	parts := strings.Fields(value)
	command := parts[0]
	m.history = append(m.history, value)
	if len(m.history) > maxHistoryEntries {
		m.history = m.history[len(m.history)-maxHistoryEntries:]
	}
	m.historyIndex = -1
	m.input.Reset()
	if m.mode == modeCommand {
		switch command {
		case "help":
			m.status = m.help()
			return nil
		case "exit", "quit":
			m.quitting = true
			return tea.Quit
		case "register":
			if len(parts) > 2 {
				m.status = "Usage: register [username]"
				return nil
			}
			username := ""
			if len(parts) == 2 {
				username = parts[1]
			}
			m.beginRegister(username)
			return nil
		case "login":
			if len(parts) > 2 {
				m.status = "Usage: login [username]"
				return nil
			}
			username := ""
			if len(parts) == 2 {
				username = parts[1]
			}
			m.beginLogin(username)
			return nil
		default:
			m.status = "Unknown command. Type help for available commands."
		}
		return nil
	}
	switch command {
	case "help":
		m.status = m.help()
	case "whoami":
		m.status = m.details()
	case "logout":
		m.logout()
	case "enable-2fa":
		return m.enableMFA()
	case "disable-2fa":
		return m.disableMFA()
	case "exit", "quit":
		m.logout()
		m.quitting = true
		return tea.Quit
	default:
		m.status = "Unknown command. Type help for available commands."
	}
	return nil
}

func (m *model) beginRegister(username string) {
	m.pendingUsername = username
	if username == "" {
		m.mode = modeRegisterUsername
		m.setInput("Username: ", textinput.EchoNormal)
		m.status = "Choose a username with 3-32 characters and no spaces."
		return
	}
	m.mode = modeRegisterPassword
	m.setInput("Password: ", textinput.EchoPassword)
	m.status = "Choose a password with at least 10 characters."
}

func (m *model) beginLogin(username string) {
	m.pendingUsername = username
	if username == "" {
		m.mode = modeLoginUsername
		m.setInput("Username: ", textinput.EchoNormal)
		m.status = "Enter your username."
		return
	}
	m.mode = modeLoginPassword
	m.setInput("Password: ", textinput.EchoPassword)
	m.status = "Enter your password."
}

func (m *model) setInput(prompt string, echo textinput.EchoMode) {
	m.input.Reset()
	m.input.Prompt = prompt
	m.input.EchoMode = echo
	m.input.Focus()
}

func (m *model) register(username, password string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		err := m.service.Register(ctx, username, password)
		if err != nil {
			return resultMsg{err: err, resetInput: true}
		}
		return resultMsg{message: "Registration complete. You can now log in.", resetInput: true}
	}
}
func (m *model) login(username, password, code string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		user, err := m.service.Login(ctx, username, password, code)
		if err != nil {
			if errors.Is(err, auth.ErrMFARequired) {
				return loginNeedsTOTPMsg{username: username, password: password}
			}
			return resultMsg{err: err, resetInput: true}
		}
		return loginMsg{user}
	}
}
func (m *model) enableMFA() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		uri, err := m.service.EnableMFA(ctx, m.user.ID, m.user.Username)
		return mfaMsg{uri, err}
	}
}
func (m *model) disableMFA() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		defer cancel()
		return resultMsg{err: m.service.DisableMFA(ctx, m.user.ID), message: "MFA disabled."}
	}
}

type resultMsg struct {
	err        error
	message    string
	resetInput bool
}
type loginMsg struct{ user auth.User }
type loginNeedsTOTPMsg struct {
	username string
	password string
}
type mfaMsg struct {
	uri string
	err error
}

func (m *model) View() string {
	if m.quitting {
		return "Goodbye.\n"
	}
	header := titleStyle.Render("OSTO / SECURE CLI") + "\n" + mutedStyle.Render("PostgreSQL-backed authentication") + "\n\n"
	m.expireSession()
	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	status := lipgloss.NewStyle().Width(contentWidth).Render(m.status)
	return header + status + "\n\n" + m.input.View() + "\n"
}

func (m *model) expireSession() {
	if m.mode == modeAuthenticated && !m.expiresAt.IsZero() && time.Now().After(m.expiresAt) {
		m.logout()
		m.status = "Session expired. Please log in again."
	}
}

func (m *model) help() string {
	if m.mode == modeCommand {
		return "Commands: register [username] | login [username] | help | exit"
	}
	return "Commands: whoami | enable-2fa | disable-2fa | logout | help | exit"
}
func (m *model) details() string {
	last := "never"
	if m.user.LastLoginAt != nil {
		last = m.user.LastLoginAt.Local().Format(time.RFC1123)
	}
	mfa := "disabled"
	if m.user.MFAEnabled {
		mfa = "enabled"
	}
	return fmt.Sprintf("user: %s | registered: %s | MFA: %s | session expires: %s | last login: %s", m.user.Username, m.user.RegisteredAt.Local().Format(time.RFC1123), mfa, m.expiresAt.Local().Format(time.RFC1123), last)
}
func (m *model) logout() {
	m.mode = modeCommand
	m.user = auth.User{}
	m.pendingUsername = ""
	m.pendingPassword = ""
	m.expiresAt = time.Time{}
	m.setInput("> ", textinput.EchoNormal)
	m.status = "Logged out."
}
func (m *model) previousHistory() {
	if len(m.history) == 0 {
		return
	}
	if m.historyIndex < 0 {
		m.historyIndex = len(m.history)
	}
	if m.historyIndex > 0 {
		m.historyIndex--
	}
	m.input.SetValue(m.history[m.historyIndex])
}
func (m *model) nextHistory() {
	if m.historyIndex < 0 {
		return
	}
	if m.historyIndex < len(m.history)-1 {
		m.historyIndex++
		m.input.SetValue(m.history[m.historyIndex])
		return
	}
	m.historyIndex = -1
	m.input.Reset()
}
func (m *model) complete() {
	commands := []string{"register", "login", "help", "exit", "quit"}
	if m.mode == modeAuthenticated {
		commands = []string{"whoami", "enable-2fa", "disable-2fa", "logout", "help", "exit"}
	}
	prefix := m.input.Value()
	for _, command := range commands {
		if strings.HasPrefix(command, prefix) {
			m.input.SetValue(command)
			break
		}
	}
}

func (m *model) applyMessage(msg tea.Msg) {
	switch msg := msg.(type) {
	case resultMsg:
		if msg.err != nil {
			m.status = errorStyle.Render(msg.err.Error())
		} else {
			m.status = okStyle.Render(msg.message)
		}
		if msg.resetInput {
			m.mode = modeCommand
			m.pendingUsername = ""
			m.pendingPassword = ""
			m.setInput("> ", textinput.EchoNormal)
		}
	case loginMsg:
		m.user = msg.user
		m.mode = modeAuthenticated
		m.pendingUsername = ""
		m.pendingPassword = ""
		m.setInput("> ", textinput.EchoNormal)
		m.expiresAt = time.Now().Add(m.sessionTimeout)
		m.status = okStyle.Render("Login successful. " + m.details())
	case loginNeedsTOTPMsg:
		m.pendingUsername = msg.username
		m.pendingPassword = msg.password
		m.mode = modeLoginTOTP
		m.setInput("MFA code: ", textinput.EchoPassword)
		m.status = "Enter the 6-digit code from your authenticator."
	case mfaMsg:
		if msg.err != nil {
			m.status = errorStyle.Render(msg.err.Error())
		} else {
			m.user.MFAEnabled = true
			m.status = okStyle.Render("MFA enabled. Add this URI to an authenticator: " + msg.uri)
		}
	}
}
