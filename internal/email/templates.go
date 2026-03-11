package email

import (
	"fmt"
	"html"
	"strings"
)

// VerificationEmail builds an email verification message.
func VerificationEmail(to, baseURL, token string) *Message {
	link := fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)
	return &Message{
		To:      to,
		Subject: "Verify your email — rstash",
		HTML: fmt.Sprintf(`<h2>Verify your email</h2>
<p>Click the link below to verify your email address:</p>
<p><a href="%s">Verify Email</a></p>
<p>This link expires in 48 hours.</p>
<p>If you didn't create an account, you can ignore this email.</p>`, link),
		Text: fmt.Sprintf("Verify your email\n\nClick the link below to verify your email address:\n\n%s\n\nThis link expires in 48 hours.\n\nIf you didn't create an account, you can ignore this email.", link),
	}
}

// PasswordResetEmail builds a password reset message.
func PasswordResetEmail(to, baseURL, token string) *Message {
	link := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
	return &Message{
		To:      to,
		Subject: "Reset your password — rstash",
		HTML: fmt.Sprintf(`<h2>Reset your password</h2>
<p>Click the link below to reset your password:</p>
<p><a href="%s">Reset Password</a></p>
<p>This link expires in 1 hour.</p>
<p>If you didn't request a password reset, you can ignore this email.</p>`, link),
		Text: fmt.Sprintf("Reset your password\n\nClick the link below to reset your password:\n\n%s\n\nThis link expires in 1 hour.\n\nIf you didn't request a password reset, you can ignore this email.", link),
	}
}

// TestEmail builds a test email message.
func TestEmail(to string) *Message {
	return &Message{
		To:      to,
		Subject: "Test email — rstash",
		HTML:    `<h2>Test Email</h2><p>If you're reading this, email delivery is working correctly.</p>`,
		Text:    "Test Email\n\nIf you're reading this, email delivery is working correctly.",
	}
}

// AnnouncementEmail builds an announcement email.
func AnnouncementEmail(to, subject, body string) *Message {
	htmlBody := strings.ReplaceAll(html.EscapeString(body), "\n", "<br>")
	return &Message{
		To:      to,
		Subject: subject,
		HTML:    fmt.Sprintf(`<div style="max-width:600px;margin:0 auto;font-family:sans-serif"><h2>%s</h2><p>%s</p></div>`, html.EscapeString(subject), htmlBody),
		Text:    body,
	}
}
