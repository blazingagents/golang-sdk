package blazingagents

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// UserMessage is a message an end user sends: a UI message with role "user". Decoding it
// checks the role, the id and the parts, and keeps fields this SDK does not know, which are
// sent again unchanged.
type UserMessage struct {
	ID       string
	Parts    []UserPart
	Metadata json.RawMessage
	extra    map[string]json.RawMessage
}

func (UserMessage) isRegenerateInput() {}

// TextMessage is a one-part text message with a random UUIDv4 id. Keep the message to retry
// a send without a message_id_conflict.
func TextMessage(text string) UserMessage {
	return NewUserMessage(newUUID(), TextPart{Text: text})
}

// NewUserMessage is a message with the given id and parts.
func NewUserMessage(id string, first UserPart, rest ...UserPart) UserMessage {
	return UserMessage{ID: id, Parts: append([]UserPart{first}, rest...)}
}

// Messages is the input of a chat call. It is never empty.
func Messages(first UserMessage, rest ...UserMessage) ChatInput {
	return messagesInput(append([]UserMessage{first}, rest...))
}

type messagesInput []UserMessage

func (messagesInput) isChatInput() {}

func (m UserMessage) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range m.extra {
		out[k] = v
	}
	out["id"], out["role"], out["parts"] = m.ID, "user", m.Parts
	if m.Metadata != nil {
		out["metadata"] = m.Metadata
	}
	return json.Marshal(out)
}

var (
	errMessageRole  = errors.New(`blazingagents: message role must be "user"`)
	errMessageID    = errors.New("blazingagents: message id must be a non-empty string")
	errMessageParts = errors.New("blazingagents: message parts must be a non-empty array")
	errPartType     = errors.New("blazingagents: message part type must be a string")
	errTextPart     = errors.New("blazingagents: text part needs a string text")
	errFilePart     = errors.New("blazingagents: file part needs string mediaType and url")
)

func (m *UserMessage) UnmarshalJSON(b []byte) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	var role, id string
	var parts []json.RawMessage
	switch {
	case json.Unmarshal(obj["role"], &role) != nil || role != "user":
		return errMessageRole
	case json.Unmarshal(obj["id"], &id) != nil || id == "":
		return errMessageID
	case json.Unmarshal(obj["parts"], &parts) != nil || len(parts) == 0:
		return errMessageParts
	}
	out := UserMessage{ID: id, Parts: make([]UserPart, len(parts)), Metadata: obj["metadata"]}
	for i, p := range parts {
		part, err := decodeUserPart(p)
		if err != nil {
			return err
		}
		out.Parts[i] = part
	}
	for _, k := range []string{"id", "role", "parts", "metadata"} {
		delete(obj, k)
	}
	if len(obj) > 0 {
		out.extra = obj
	}
	*m = out
	return nil
}

// UserPart is a part of a UserMessage: TextPart, FilePart or UnknownPart.
type UserPart interface{ isUserPart() }

// TextPart is text. Fields this SDK does not know (providerMetadata, state, ...) are kept on
// decode and sent again.
type TextPart struct {
	Text  string
	extra map[string]json.RawMessage
}

// FilePart is a file by URL. Unknown fields are kept on decode and sent again.
type FilePart struct {
	MediaType string
	URL       string
	Filename  string // "" sends no filename
	extra     map[string]json.RawMessage
}

// UnknownPart is a part type this SDK does not know. It is sent again as Raw.
type UnknownPart struct {
	Type string
	Raw  json.RawMessage
}

func (TextPart) isUserPart()    {}
func (FilePart) isUserPart()    {}
func (UnknownPart) isUserPart() {}

func (p TextPart) MarshalJSON() ([]byte, error) {
	return marshalPart(p.extra, map[string]any{"type": "text", "text": p.Text})
}

func (p FilePart) MarshalJSON() ([]byte, error) {
	fields := map[string]any{"type": "file", "mediaType": p.MediaType, "url": p.URL}
	if p.Filename != "" {
		fields["filename"] = p.Filename
	}
	return marshalPart(p.extra, fields)
}

func (p UnknownPart) MarshalJSON() ([]byte, error) { return p.Raw, nil }

func marshalPart(extra map[string]json.RawMessage, fields map[string]any) ([]byte, error) {
	for k, v := range extra {
		if _, ok := fields[k]; !ok {
			fields[k] = v
		}
	}
	return json.Marshal(fields)
}

func decodeUserPart(raw json.RawMessage) (UserPart, error) {
	var obj map[string]json.RawMessage
	var typ string
	if json.Unmarshal(raw, &obj) != nil || json.Unmarshal(obj["type"], &typ) != nil {
		return nil, errPartType
	}
	extra := func(known ...string) map[string]json.RawMessage {
		for _, k := range append(known, "type") {
			delete(obj, k)
		}
		if len(obj) == 0 {
			return nil
		}
		return obj
	}
	switch typ {
	case "text":
		var p TextPart
		if json.Unmarshal(obj["text"], &p.Text) != nil {
			return nil, errTextPart
		}
		p.extra = extra("text")
		return p, nil
	case "file":
		var p FilePart
		if json.Unmarshal(obj["mediaType"], &p.MediaType) != nil || json.Unmarshal(obj["url"], &p.URL) != nil {
			return nil, errFilePart
		}
		if f, ok := obj["filename"]; ok && json.Unmarshal(f, &p.Filename) != nil {
			return nil, errFilePart
		}
		p.extra = extra("mediaType", "url", "filename")
		return p, nil
	}
	return UnknownPart{Type: typ, Raw: append(json.RawMessage(nil), raw...)}, nil
}

// Message is a stored transcript message, of a session or a task run.
type Message struct {
	ID         string            `json:"id"`
	Role       MessageRole       `json:"role"`
	Branchable bool              `json:"branchable"`
	Parts      []json.RawMessage `json:"parts"`
	Metadata   json.RawMessage   `json:"metadata,omitzero"`
}

// newUUID is a random UUIDv4.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails since Go 1.24
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
