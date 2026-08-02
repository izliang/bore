package amis

// AuthMessages 定义鉴权流程中所有面向用户的文案。
//
// 零值不可用，请使用 DefaultAuthMessages() 或自定义。
//
// 支持通过 AuthConfig.Messages 字段替换，实现国际化。
type AuthMessages struct {
	// ── 登录页面 ──────────────────────────────────

	// LoginTitle 登录页面标题。
	LoginTitle string

	// UsernameLabel 用户名输入框标签。
	UsernameLabel string

	// UsernamePlaceholder 用户名输入框占位提示。
	UsernamePlaceholder string

	// PasswordLabel 密码输入框标签。
	PasswordLabel string

	// PasswordPlaceholder 密码输入框占位提示。
	PasswordPlaceholder string

	// LoginButton 登录按钮文字。
	LoginButton string

	// ── 错误消息 ──────────────────────────────────

	// ErrUnauthorized 未登录/登录过期时的提示。
	ErrUnauthorized string

	// ErrMissingCredentials 用户名或密码为空时的提示。
	ErrMissingCredentials string

	// ── 登出 ──────────────────────────────────────

	// LoggedOut 登出成功时的提示。
	LoggedOut string
}

// DefaultAuthMessages 返回英文默认文案。
func DefaultAuthMessages() AuthMessages {
	return AuthMessages{
		LoginTitle:           "Login",
		UsernameLabel:        "Username",
		UsernamePlaceholder:  "Enter username",
		PasswordLabel:        "Password",
		PasswordPlaceholder:  "Enter password",
		LoginButton:          "Sign In",
		ErrUnauthorized:      "Not logged in or session expired",
		ErrMissingCredentials: "Please enter username and password",
		LoggedOut:            "Logged out",
	}
}

// AuthMessagesZH 返回中文文案，适用于简体中文环境。
func AuthMessagesZH() AuthMessages {
	return AuthMessages{
		LoginTitle:           "登录",
		UsernameLabel:        "用户名",
		UsernamePlaceholder:  "请输入用户名",
		PasswordLabel:        "密码",
		PasswordPlaceholder:  "请输入密码",
		LoginButton:          "登 录",
		ErrUnauthorized:      "未登录或登录已过期",
		ErrMissingCredentials: "请输入用户名和密码",
		LoggedOut:            "已登出",
	}
}

// normalize 填充空字段为英文默认值。
func (m *AuthMessages) normalize() {
	d := DefaultAuthMessages()
	if m.LoginTitle == "" {
		m.LoginTitle = d.LoginTitle
	}
	if m.UsernameLabel == "" {
		m.UsernameLabel = d.UsernameLabel
	}
	if m.UsernamePlaceholder == "" {
		m.UsernamePlaceholder = d.UsernamePlaceholder
	}
	if m.PasswordLabel == "" {
		m.PasswordLabel = d.PasswordLabel
	}
	if m.PasswordPlaceholder == "" {
		m.PasswordPlaceholder = d.PasswordPlaceholder
	}
	if m.LoginButton == "" {
		m.LoginButton = d.LoginButton
	}
	if m.ErrUnauthorized == "" {
		m.ErrUnauthorized = d.ErrUnauthorized
	}
	if m.ErrMissingCredentials == "" {
		m.ErrMissingCredentials = d.ErrMissingCredentials
	}
	if m.LoggedOut == "" {
		m.LoggedOut = d.LoggedOut
	}
}
