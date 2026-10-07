package email

import (
	"context"
	"net/smtp"
	"strings"
	"testing"
)

func TestNotifyReplyNamesSubjectAndLinkOnly(t *testing.T) {
	n, err := NewSMTPNotifier(SMTPConfig{Addr: "smtp.example.com:587", Username: "u", Password: "p", From: "Dupli1 <no-reply@dupli1.com>"})
	if err != nil {
		t.Fatalf("NewSMTPNotifier: %v", err)
	}
	var (
		gotTo   []string
		gotMsg  string
		gotAuth smtp.Auth
	)
	n.send = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotTo, gotMsg, gotAuth = to, string(msg), a
		return nil
	}

	err = n.NotifyReply(context.Background(), "shopper@example.com", "Prada Galleria", "https://dupli1.com/profile/support")
	if err != nil {
		t.Fatalf("NotifyReply: %v", err)
	}
	if len(gotTo) != 1 || gotTo[0] != "shopper@example.com" || gotAuth == nil {
		t.Fatalf("to = %v auth = %v", gotTo, gotAuth)
	}
	for _, want := range []string{
		"Prada Galleria", "https://dupli1.com/profile/support", "charset=UTF-8",
		"상담원이 답변을 남겼습니다", "Our team has replied to your question about \"Prada Galleria\"",
	} {
		if !strings.Contains(gotMsg, want) {
			t.Errorf("message lacks %q:\n%s", want, gotMsg)
		}
	}
}

func TestNotifyReplyRejectsHeaderInjection(t *testing.T) {
	n, _ := NewSMTPNotifier(SMTPConfig{Addr: "relay:25", From: "no-reply@dupli1.com"})
	n.send = func(string, smtp.Auth, string, []string, []byte) error { return nil }
	if err := n.NotifyReply(context.Background(), "a@b.com\r\nBcc: x@y.com", "", "https://x"); err == nil {
		t.Fatal("expected a malformed recipient to be refused")
	}
	if err := n.NotifyReply(context.Background(), "a@b.com", "", "https://x\r\nBcc: x@y.com"); err == nil {
		t.Fatal("expected a multi-line link to be refused")
	}
}
