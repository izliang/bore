package amis

import (
	"strings"
	"testing"
)

func TestDefaultAuthMessages(t *testing.T) {
	m := DefaultAuthMessages()
	if m.LoginTitle != "Login" {
		t.Errorf("LoginTitle = %q, want \"Login\"", m.LoginTitle)
	}
	if m.LoginButton != "Sign In" {
		t.Errorf("LoginButton = %q, want \"Sign In\"", m.LoginButton)
	}
}

func TestAuthMessagesZH(t *testing.T) {
	m := AuthMessagesZH()
	if !strings.Contains(m.LoginTitle, "登录") {
		t.Errorf("LoginTitle should contain Chinese, got %q", m.LoginTitle)
	}
}

func TestAuthMessagesNormalize(t *testing.T) {
	m := AuthMessages{} // zero value
	m.normalize()
	if m.LoginTitle != "Login" {
		t.Errorf("normalized LoginTitle = %q", m.LoginTitle)
	}
}

func TestLoginSchemaUsesMessages(t *testing.T) {
	cfg := DefaultAuthConfig()
	msgs := AuthMessagesZH()
	cfg.Messages = &msgs
	schema := cfg.LoginSchema()

	title, _ := schema["title"].(string)
	if title != "登录" {
		t.Errorf("schema title = %q, want Chinese", title)
	}

	// Check form body labels
	body := schema["body"].(map[string]interface{})
	form := body["body"].(map[string]interface{})
	fields := form["body"].([]interface{})

	userField := fields[0].(map[string]interface{})
	if userField["label"] != "用户名" {
		t.Errorf("username label = %v", userField["label"])
	}

	pwField := fields[1].(map[string]interface{})
	if pwField["label"] != "密码" {
		t.Errorf("password label = %v", pwField["label"])
	}
}
