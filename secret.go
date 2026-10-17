package blazingagents

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

const redacted = "[redacted]"

// Secret is a credential. Every fmt verb, log/slog handler and encoding/json print
// "[redacted]"; Reveal returns the value. The zero Secret is absent and is omitted
// from a request; NewSecret("") is present and empty.
//
// Two Secrets compare equal only when one is a copy of the other; compare Reveal results instead.
type Secret struct {
	// The value sits behind a pointer: where fmt turns methods off (a verb that does not fit,
	// such as %s on a pointer to a struct holding a Secret), it prints the address, not the value.
	p *string
}

// NewSecret wraps a credential.
func NewSecret(s string) Secret { return Secret{&s} }

// Reveal returns the credential.
func (s Secret) Reveal() string {
	if s.p == nil {
		return ""
	}
	return *s.p
}

// IsZero reports whether the Secret is absent.
func (s Secret) IsZero() bool { return s.p == nil }

func (Secret) String() string               { return redacted }
func (Secret) GoString() string             { return redacted }
func (Secret) Format(f fmt.State, _ rune)   { io.WriteString(f, redacted) }
func (Secret) LogValue() slog.Value         { return slog.StringValue(redacted) }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// UnmarshalJSON reads a JSON string; null leaves the Secret absent.
func (s *Secret) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	s.p = &v
	return nil
}

// wirer is implemented by every params type that holds a Secret at any depth.
type wirer interface {
	// wire returns the body to marshal, with secrets revealed, and the secrets for redaction.
	wire() (body any, secrets []string)
}
