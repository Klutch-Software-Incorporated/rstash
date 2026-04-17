package webhooks

// Event is the string identifier for a webhook event delivered to subscribers.
//
// IMPORTANT: the string VALUES below are the external contract. Subscribers
// filter by these strings and match against them in their own code, so the
// constant *values* must stay stable across versions. You may rename the Go
// constant identifiers freely (e.g. EventUserClaimed -> EventAccountClaimed)
// but do not change the quoted string unless you intend a breaking change
// that requires every subscriber to update.
type Event string

const (
	EventUserClaimed      Event = "user.claimed"
	EventUserEmailChanged Event = "user.email_changed"
	EventUserDisabled     Event = "user.disabled"
	EventUserEnabled      Event = "user.enabled"
	EventUserDeleted      Event = "user.deleted"
)

// String returns the wire value for an Event.
func (e Event) String() string { return string(e) }

// AllEvents returns the closed set of webhook events available in this build.
// The admin UI renders a subscribe checkbox per entry, and the subscription
// validator uses this as the allow-list when parsing the events form field.
func AllEvents() []Event {
	return []Event{
		EventUserClaimed,
		EventUserEmailChanged,
		EventUserDisabled,
		EventUserEnabled,
		EventUserDeleted,
	}
}

// IsKnown reports whether the given string is one of this build's known events.
// Unknown values in a subscription's events field (e.g. from an older-version
// admin UI that named an event that has since been renamed) are silently
// ignored during delivery fan-out.
func IsKnown(name string) bool {
	for _, e := range AllEvents() {
		if string(e) == name {
			return true
		}
	}
	return false
}
