package email

import "context"

// Message represents an email to be sent.
type Message struct {
	To      string
	From    string
	Subject string
	HTML    string
	Text    string
}

// Mailer is the interface for sending email.
type Mailer interface {
	Send(ctx context.Context, msg *Message) error
}
