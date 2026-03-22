package email

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"

	"github.com/proxera/backend/internal/crypto"
	"github.com/proxera/backend/internal/settings"
)

// SendVerificationEmail sends an email verification link to the user.
func SendVerificationEmail(toEmail, userName, token string) error {
	if settings.Get("ENABLE_EMAIL_VERIFICATION", "false") != "true" {
		return nil
	}

	siteURL := settings.Get("PUBLIC_SITE_URL", "http://localhost:8080")
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", siteURL, token)

	subject := "Verify your Proxera account"
	body := fmt.Sprintf(`Hi %s,

Thanks for signing up for Proxera! Please verify your email address by clicking the link below:

%s

This link expires in 24 hours.

If you didn't create this account, you can safely ignore this email.

— Proxera`, userName, verifyURL)

	return sendMail(toEmail, subject, body)
}

// SendPasswordResetEmail sends a password reset link to the user.
func SendPasswordResetEmail(toEmail, userName, resetURL string) error {
	subject := "Reset your Proxera password"
	body := fmt.Sprintf(`Hi %s,

A password reset was requested for your Proxera account. Click the link below to set a new password:

%s

This link expires in 1 hour. If you did not request a password reset, you can safely ignore this email.

— Proxera`, userName, resetURL)

	return sendMail(toEmail, subject, body)
}

func sendMail(to, subject, body string) error {
	host := settings.Get("SMTP_HOST", "")
	port := settings.Get("SMTP_PORT", "465")
	user := settings.Get("SMTP_USER", "")
	pass := settings.Get("SMTP_PASSWORD", "")
	from := settings.Get("SMTP_FROM_EMAIL", "")

	// Attempt to decrypt the SMTP password. If decryption fails, the value is
	// either a plaintext password from an env var or a pre-encryption DB entry.
	// In both cases, use the raw value for backward compatibility.
	if pass != "" {
		if decrypted, err := crypto.Decrypt(pass); err == nil {
			pass = decrypted
		}
	}

	if host == "" || port == "" || user == "" || pass == "" {
		return fmt.Errorf("SMTP not configured (missing SMTP_HOST/PORT/USER/PASSWORD)")
	}

	if from == "" {
		from = user
	}

	// Strip display name for envelope sender: "Name <addr>" → "addr"
	envelopeFrom := from
	if idx := strings.Index(from, "<"); idx != -1 {
		envelopeFrom = strings.Trim(from[idx:], "<> ")
	}

	addr := net.JoinHostPort(host, port)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	auth := smtp.PlainAuth("", user, pass, host)

	// Port 465 uses implicit TLS (SMTPS), port 587 uses STARTTLS
	if port == "465" {
		return sendSMTPS(addr, host, auth, envelopeFrom, to, msg)
	}
	return smtp.SendMail(addr, auth, envelopeFrom, []string{to}, []byte(msg))
}

// sendSMTPS handles implicit TLS connections (port 465)
func sendSMTPS(addr, host string, auth smtp.Auth, from, to, msg string) error {
	tlsConfig := &tls.Config{ServerName: host}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}

	if err = client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}

	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}

	if _, err = w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("SMTP close data failed: %w", err)
	}

	if err = client.Quit(); err != nil {
		slog.Warn("SMTP QUIT warning", "component", "email", "error", err)
	}

	return nil
}
