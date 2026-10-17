package testkit

import (
	"context"
	"strings"
	"testing"

	ba "github.com/blazingagents/golang-sdk"
)

// Case is one table-driven call against the mock.
type Case struct {
	Name      string
	Exchanges []Exchange // served in order
	Call      func(ctx context.Context, c *ba.Client) (any, error)
	Example   string                      // when set, the result must MatchesExample it
	Unknown   bool                        // the last exchange carries an unknown union tag: UnknownRoundTrip it
	Check     func(t *testing.T, got any) // further assertions
	Err       string                      // when set, the call must fail with exactly this message
	ErrPrefix string                      // when set, the call's error message must start with this
}

// Run runs each case as a subtest with its own mock and tenant client.
func Run(t *testing.T, cases []Case) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			m := New(t)
			for _, e := range c.Exchanges {
				m.Expect(e)
			}
			got, err := c.Call(context.Background(), m.Client())
			switch {
			case c.ErrPrefix != "":
				if err == nil || !strings.HasPrefix(err.Error(), c.ErrPrefix) {
					t.Fatalf("error %v, want prefix %s", err, c.ErrPrefix)
				}
				return
			case c.Err != "":
				if err == nil || err.Error() != c.Err {
					t.Fatalf("error %v, want %s", err, c.Err)
				}
				return
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			}
			if c.Example != "" {
				MatchesExample(t, c.Example, got)
			}
			if c.Unknown {
				UnknownRoundTrip(t, c.Exchanges[len(c.Exchanges)-1], got)
			}
			if c.Check != nil {
				c.Check(t, got)
			}
		})
	}
}
