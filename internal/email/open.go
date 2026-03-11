package email

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseDSN extracts the provider name and from address from an email DSN.
// Returns empty strings if the DSN is empty or unparseable.
func ParseDSN(dsn string) (provider, from string) {
	if dsn == "" {
		return "", ""
	}
	idx := strings.Index(dsn, ":")
	if idx < 1 {
		return "", ""
	}
	scheme := dsn[:idx]
	rest := dsn[idx+1:]

	switch scheme {
	case "resend":
		provider = "Resend"
	default:
		provider = scheme
	}

	if qIdx := strings.Index(rest, "?"); qIdx >= 0 {
		params, err := url.ParseQuery(rest[qIdx+1:])
		if err == nil {
			from = params.Get("from")
		}
	}
	return provider, from
}

// Open creates a Mailer from a DSN string.
// DSN format: "resend:API_KEY?from=noreply@example.com"
// Returns nil if dsn is empty (email not configured).
func Open(dsn string) (Mailer, error) {
	if dsn == "" {
		return nil, nil
	}

	idx := strings.Index(dsn, ":")
	if idx < 1 {
		return nil, fmt.Errorf("invalid email DSN %q: missing scheme (expected scheme:value)", dsn)
	}

	scheme := dsn[:idx]
	rest := dsn[idx+1:]

	switch scheme {
	case "resend":
		return openResend(rest)
	default:
		return nil, fmt.Errorf("unsupported email scheme %q", scheme)
	}
}

func openResend(rest string) (Mailer, error) {
	// rest is "API_KEY?from=noreply@example.com"
	apiKey := rest
	var from string

	if qIdx := strings.Index(rest, "?"); qIdx >= 0 {
		apiKey = rest[:qIdx]
		params, err := url.ParseQuery(rest[qIdx+1:])
		if err != nil {
			return nil, fmt.Errorf("parse resend DSN params: %w", err)
		}
		from = params.Get("from")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("resend API key is required")
	}
	if from == "" {
		return nil, fmt.Errorf("resend 'from' address is required (add ?from=noreply@example.com)")
	}

	return NewResendMailer(apiKey, from), nil
}
