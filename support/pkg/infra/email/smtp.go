// Package email sends the reply notice a web shopper gets when a manager's
// answer has gone unread (docs/support-web-chat.md).
package email

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SMTPNotifier sends notices through an SMTP relay.
type SMTPNotifier struct {
	addr     string
	host     string
	username string
	password string
	from     mail.Address
	now      func() time.Time
	send     func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// SMTPConfig is the relay and sender. Addr is host:port; Username/Password
// may be empty for an unauthenticated relay on a private network.
type SMTPConfig struct {
	Addr     string
	Username string
	Password string
	From     string
}

// NewSMTPNotifier validates the config. smtp.SendMail upgrades to STARTTLS
// whenever the server offers it, and refuses PLAIN auth on a cleartext
// connection to anything but localhost, so a password is never sent in the
// clear.
func NewSMTPNotifier(cfg SMTPConfig) (*SMTPNotifier, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(cfg.Addr))
	if err != nil {
		return nil, fmt.Errorf("smtp address %q: %w", cfg.Addr, err)
	}
	from, err := mail.ParseAddress(strings.TrimSpace(cfg.From))
	if err != nil {
		return nil, fmt.Errorf("smtp from %q: %w", cfg.From, err)
	}
	return &SMTPNotifier{
		addr:     strings.TrimSpace(cfg.Addr),
		host:     host,
		username: cfg.Username,
		password: cfg.Password,
		from:     *from,
		now:      time.Now,
		send:     smtp.SendMail,
	}, nil
}

// NotifyReply sends the notice. It names what the consultation is about and
// links to the chat — never the reply itself.
func (n *SMTPNotifier) NotifyReply(_ context.Context, to, subject, link string) error {
	recipient, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return fmt.Errorf("recipient: %w", err)
	}
	if strings.ContainsAny(link, "\r\n") {
		return errors.New("link must be a single line")
	}

	var auth smtp.Auth
	if n.username != "" {
		auth = smtp.PlainAuth("", n.username, n.password, n.host)
	}
	msg := n.compose(recipient.Address, subject, link)
	if err := n.send(n.addr, auth, n.from.Address, []string{recipient.Address}, msg); err != nil {
		return fmt.Errorf("send reply notice: %w", err)
	}
	return nil
}

func (n *SMTPNotifier) compose(to, subject, link string) []byte {
	about := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(subject, "\r", " "), "\n", " "))

	// Korean first, then English: the notice goes out without knowing which
	// language the shopper reads the storefront in.
	var body strings.Builder
	body.WriteString("안녕하세요, Dupli1입니다.\r\n\r\n")
	if about != "" {
		body.WriteString("문의하신 \"" + about + "\"에 대해 상담원이 답변을 남겼습니다.\r\n")
	} else {
		body.WriteString("문의하신 내용에 대해 상담원이 답변을 남겼습니다.\r\n")
	}
	body.WriteString("아래 링크에서 답변을 확인해 주세요.\r\n\r\n")
	body.WriteString(link + "\r\n\r\n")
	body.WriteString("본 메일은 발신 전용입니다. 문의는 채팅 상담으로 이어서 남겨 주세요.\r\n")
	body.WriteString("\r\n----------\r\n\r\n")
	body.WriteString("Hello from Dupli1.\r\n\r\n")
	if about != "" {
		body.WriteString("Our team has replied to your question about \"" + about + "\".\r\n")
	} else {
		body.WriteString("Our team has replied to your question.\r\n")
	}
	body.WriteString("Open the link below to read the reply.\r\n\r\n")
	body.WriteString(link + "\r\n\r\n")
	body.WriteString("This address does not receive mail. To follow up, continue on the site.\r\n")

	var msg strings.Builder
	msg.WriteString("From: " + n.from.String() + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + mime.BEncoding.Encode("UTF-8", "[Dupli1] 상담원이 답변했습니다 / You have a reply") + "\r\n")
	msg.WriteString("Date: " + n.now().Format(time.RFC1123Z) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body.String())
	return []byte(msg.String())
}
