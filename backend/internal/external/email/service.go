package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// Sender is the interface for sending emails. Implementations can be swapped
// (e.g. replace Gmail SMTP with a dedicated email API) without touching callers.
type Sender interface {
	Send(to, subject, body string) error
}

// GmailService sends email via Gmail SMTP using an app password.
type GmailService struct {
	fromAddress string
	appPassword string
}

// NewGmailService creates a GmailService. fromAddress is the Gmail account
// address; appPassword is a Google app password (not the account password).
func NewGmailService(fromAddress, appPassword string) *GmailService {
	return &GmailService{fromAddress: fromAddress, appPassword: appPassword}
}

// Send delivers a plain-text email to the given recipient via Gmail SMTP with
// STARTTLS on port 587.
func (g *GmailService) Send(to, subject, body string) error {
	host := "smtp.gmail.com"
	port := "587"
	addr := net.JoinHostPort(host, port)

	msg := buildMessage(g.fromAddress, to, subject, body)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("email: dial smtp: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("email: create smtp client: %w", err)
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
		return fmt.Errorf("email: starttls: %w", err)
	}

	auth := smtp.PlainAuth("", g.fromAddress, g.appPassword, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("email: auth: %w", err)
	}

	if err := client.Mail(g.fromAddress); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := fmt.Fprint(w, msg); err != nil {
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close data writer: %w", err)
	}

	return client.Quit()
}

// IsTransient reports whether err is a transient SMTP failure that warrants a
// retry. Network errors and SMTP 4xx responses are transient; SMTP 5xx and
// authentication failures are permanent.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// SMTP 4xx = temporary failure (rate limit, try again later)
	if strings.Contains(msg, "450") || strings.Contains(msg, "451") ||
		strings.Contains(msg, "452") {
		return true
	}
	// Network / connection errors are transient
	var netErr net.Error
	if ok := isNetError(err, &netErr); ok {
		return true
	}
	return false
}

func isNetError(err error, target *net.Error) bool {
	e, ok := err.(net.Error)
	if ok {
		*target = e
	}
	return ok
}

func buildMessage(from, to, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}
