package blazingagents

import (
	"encoding/json"
	"time"
)

func absent(raw json.RawMessage) bool { return len(raw) == 0 || string(raw) == "null" }

// Schedule is when a task runs: ScheduleOnce, ScheduleInterval, ScheduleCron or UnknownSchedule.
type Schedule interface{ isSchedule() }

// ScheduleOnce runs a task once, at At. A zero At fails to marshal.
type ScheduleOnce struct{ At time.Time }

// ScheduleInterval runs a task every Every; the server's minimum is one minute.
type ScheduleInterval struct{ Every time.Duration }

// ScheduleCron runs a task on a cron expression.
type ScheduleCron struct {
	Expression string
	Timezone   *string        // server default "UTC"
	Stagger    *time.Duration // staggerMs on the wire
}

// UnknownSchedule is a schedule kind this SDK does not know. It is sent again as Raw.
type UnknownSchedule struct {
	Kind string
	Raw  json.RawMessage
}

func (ScheduleOnce) isSchedule()     {}
func (ScheduleInterval) isSchedule() {}
func (ScheduleCron) isSchedule()     {}
func (UnknownSchedule) isSchedule()  {}

type scheduleOnceWire struct {
	At time.Time `json:"at"`
}

type scheduleIntervalWire struct {
	EveryMs int64 `json:"everyMs"`
}

type scheduleCronWire struct {
	Expression string  `json:"expression"`
	Timezone   *string `json:"timezone,omitzero"`
	StaggerMs  *int64  `json:"staggerMs,omitzero"`
}

var errZeroScheduleAt = &ArgumentError{Name: "ScheduleOnce.At", Reason: "zero"}

func (s ScheduleOnce) MarshalJSON() ([]byte, error) {
	if s.At.IsZero() {
		return nil, errZeroScheduleAt
	}
	return encodeVariant("Schedule", "once", scheduleOnceWire{s.At.UTC()})
}

func (s ScheduleInterval) MarshalJSON() ([]byte, error) {
	return encodeVariant("Schedule", "interval", scheduleIntervalWire{s.Every.Milliseconds()})
}

func (s ScheduleCron) MarshalJSON() ([]byte, error) {
	w := scheduleCronWire{Expression: s.Expression, Timezone: s.Timezone}
	if s.Stagger != nil {
		w.StaggerMs = new(s.Stagger.Milliseconds())
	}
	return encodeVariant("Schedule", "cron", w)
}

func (s UnknownSchedule) MarshalJSON() ([]byte, error) { return s.Raw, nil }

var scheduleVariants = map[string]func([]byte) (Schedule, error){
	"once": variant(func(w scheduleOnceWire) Schedule { return ScheduleOnce(w) }),
	"interval": variant(func(w scheduleIntervalWire) Schedule {
		return ScheduleInterval{Every: time.Duration(w.EveryMs) * time.Millisecond}
	}),
	"cron": variant(func(w scheduleCronWire) Schedule {
		s := ScheduleCron{Expression: w.Expression, Timezone: w.Timezone}
		if w.StaggerMs != nil {
			s.Stagger = new(time.Duration(*w.StaggerMs) * time.Millisecond)
		}
		return s
	}),
}

func decodeSchedule(raw json.RawMessage) (Schedule, error) {
	if absent(raw) {
		return nil, nil
	}
	return decodeUnion("Schedule", raw, scheduleVariants, func(tag string, raw json.RawMessage) Schedule {
		return UnknownSchedule{Kind: tag, Raw: raw}
	})
}

// NetworkPolicy is a workspace's network access: NetworkPolicyUnrestricted,
// NetworkPolicyAllowlist, NetworkPolicyOffline or UnknownNetworkPolicy.
type NetworkPolicy interface{ isNetworkPolicy() }

// NetworkPolicyUnrestricted allows every host.
type NetworkPolicyUnrestricted struct{}

// NetworkPolicyAllowlist allows only AllowedHosts, which must not be empty.
type NetworkPolicyAllowlist struct{ AllowedHosts []string }

// NetworkPolicyOffline allows no network access.
type NetworkPolicyOffline struct{}

// UnknownNetworkPolicy is a mode this SDK does not know. It is sent again as Raw.
type UnknownNetworkPolicy struct {
	Mode string
	Raw  json.RawMessage
}

func (NetworkPolicyUnrestricted) isNetworkPolicy() {}
func (NetworkPolicyAllowlist) isNetworkPolicy()    {}
func (NetworkPolicyOffline) isNetworkPolicy()      {}
func (UnknownNetworkPolicy) isNetworkPolicy()      {}

type allowlistWire struct {
	AllowedHosts []string `json:"allowedHosts"`
}

var errEmptyAllowlist = &ArgumentError{Name: "NetworkPolicyAllowlist.AllowedHosts", Reason: "empty"}

func (NetworkPolicyUnrestricted) MarshalJSON() ([]byte, error) {
	return encodeVariant("NetworkPolicy", "unrestricted", struct{}{})
}

func (p NetworkPolicyAllowlist) MarshalJSON() ([]byte, error) {
	if len(p.AllowedHosts) == 0 {
		return nil, errEmptyAllowlist
	}
	return encodeVariant("NetworkPolicy", "allowlist", allowlistWire(p))
}

func (NetworkPolicyOffline) MarshalJSON() ([]byte, error) {
	return encodeVariant("NetworkPolicy", "offline", struct{}{})
}

func (p UnknownNetworkPolicy) MarshalJSON() ([]byte, error) { return p.Raw, nil }

var networkPolicyVariants = map[string]func([]byte) (NetworkPolicy, error){
	"unrestricted": variant(func(struct{}) NetworkPolicy { return NetworkPolicyUnrestricted{} }),
	"allowlist":    variant(func(w allowlistWire) NetworkPolicy { return NetworkPolicyAllowlist(w) }),
	"offline":      variant(func(struct{}) NetworkPolicy { return NetworkPolicyOffline{} }),
}

func decodeNetworkPolicy(raw json.RawMessage) (NetworkPolicy, error) {
	if absent(raw) {
		return nil, nil
	}
	return decodeUnion("NetworkPolicy", raw, networkPolicyVariants, func(tag string, raw json.RawMessage) NetworkPolicy {
		return UnknownNetworkPolicy{Mode: tag, Raw: raw}
	})
}

// ToolReference names a tool: ToolReferenceBuiltin, ToolReferenceMCP, ToolReferenceFunction
// or UnknownToolReference.
type ToolReference interface{ isToolReference() }

// ToolReferenceBuiltin is a builtin tool.
type ToolReferenceBuiltin struct{ Name AgentTool }

// ToolReferenceMCP is a tool of an MCP connection.
type ToolReferenceMCP struct {
	ConnectionID MCPConnectionID
	Name         string
}

// ToolReferenceFunction is a backend function.
type ToolReferenceFunction struct{ Name string }

// UnknownToolReference is a tool type this SDK does not know. It is sent again as Raw.
type UnknownToolReference struct {
	Type string
	Raw  json.RawMessage
}

func (ToolReferenceBuiltin) isToolReference()  {}
func (ToolReferenceMCP) isToolReference()      {}
func (ToolReferenceFunction) isToolReference() {}
func (UnknownToolReference) isToolReference()  {}

type builtinToolWire struct {
	Name AgentTool `json:"name"`
}

type mcpToolWire struct {
	ConnectionID MCPConnectionID `json:"connectionId"`
	Name         string          `json:"name"`
}

type functionToolWire struct {
	Name string `json:"name"`
}

func (t ToolReferenceBuiltin) MarshalJSON() ([]byte, error) {
	return encodeVariant("ToolReference", "builtin", builtinToolWire(t))
}

func (t ToolReferenceMCP) MarshalJSON() ([]byte, error) {
	return encodeVariant("ToolReference", "mcp", mcpToolWire(t))
}

func (t ToolReferenceFunction) MarshalJSON() ([]byte, error) {
	return encodeVariant("ToolReference", "function", functionToolWire(t))
}

func (t UnknownToolReference) MarshalJSON() ([]byte, error) { return t.Raw, nil }

var toolReferenceVariants = map[string]func([]byte) (ToolReference, error){
	"builtin":  variant(func(w builtinToolWire) ToolReference { return ToolReferenceBuiltin(w) }),
	"mcp":      variant(func(w mcpToolWire) ToolReference { return ToolReferenceMCP(w) }),
	"function": variant(func(w functionToolWire) ToolReference { return ToolReferenceFunction(w) }),
}

func decodeToolReference(raw json.RawMessage) (ToolReference, error) {
	if absent(raw) {
		return nil, nil
	}
	return decodeUnion("ToolReference", raw, toolReferenceVariants, func(tag string, raw json.RawMessage) ToolReference {
		return UnknownToolReference{Type: tag, Raw: raw}
	})
}

// ChatConfiguration is a chat connection's platform settings: SlackConfiguration,
// TelegramConfiguration or UnknownChatConfiguration.
type ChatConfiguration interface{ isChatConfiguration() }

// SlackConfiguration lists the Slack channels the connection serves.
type SlackConfiguration struct{ ChannelIDs []string }

// TelegramConfiguration lists the Telegram chats the connection serves.
type TelegramConfiguration struct {
	ChatIDs      []string
	BusinessMode bool
}

// UnknownChatConfiguration is a platform this SDK does not know. It is sent again as Raw.
type UnknownChatConfiguration struct {
	Platform string
	Raw      json.RawMessage
}

func (SlackConfiguration) isChatConfiguration()       {}
func (TelegramConfiguration) isChatConfiguration()    {}
func (UnknownChatConfiguration) isChatConfiguration() {}

type slackConfigurationWire struct {
	ChannelIDs []string `json:"channelIds,omitzero"`
}

type telegramConfigurationWire struct {
	ChatIDs      []string `json:"chatIds,omitzero"`
	BusinessMode bool     `json:"businessMode"`
}

func (c SlackConfiguration) MarshalJSON() ([]byte, error) {
	return encodeVariant("ChatConfiguration", "slack", slackConfigurationWire(c))
}

func (c TelegramConfiguration) MarshalJSON() ([]byte, error) {
	return encodeVariant("ChatConfiguration", "telegram", telegramConfigurationWire(c))
}

func (c UnknownChatConfiguration) MarshalJSON() ([]byte, error) { return c.Raw, nil }

var chatConfigurationVariants = map[string]func([]byte) (ChatConfiguration, error){
	"slack":    variant(func(w slackConfigurationWire) ChatConfiguration { return SlackConfiguration(w) }),
	"telegram": variant(func(w telegramConfigurationWire) ChatConfiguration { return TelegramConfiguration(w) }),
}

func decodeChatConfiguration(raw json.RawMessage) (ChatConfiguration, error) {
	if absent(raw) {
		return nil, nil
	}
	return decodeUnion("ChatConfiguration", raw, chatConfigurationVariants, func(tag string, raw json.RawMessage) ChatConfiguration {
		return UnknownChatConfiguration{Platform: tag, Raw: raw}
	})
}

// SkillCopyResult is the outcome of copying a skill to one agent: SkillCopyCreated,
// SkillCopyFailed or UnknownSkillCopyResult.
type SkillCopyResult interface{ isSkillCopyResult() }

// SkillCopyCreated is a copy that was created.
type SkillCopyCreated struct {
	AgentID AgentID
	Skill   Skill
}

// SkillCopyFailed is a copy that failed.
type SkillCopyFailed struct {
	AgentID AgentID
	Error   SkillCopyError
}

// UnknownSkillCopyResult is a status this SDK does not know. It is sent again as Raw.
type UnknownSkillCopyResult struct {
	Status string
	Raw    json.RawMessage
}

func (SkillCopyCreated) isSkillCopyResult()       {}
func (SkillCopyFailed) isSkillCopyResult()        {}
func (UnknownSkillCopyResult) isSkillCopyResult() {}

type skillCopyCreatedWire struct {
	AgentID AgentID `json:"agentId"`
	Skill   Skill   `json:"skill"`
}

type skillCopyFailedWire struct {
	AgentID AgentID        `json:"agentId"`
	Error   SkillCopyError `json:"error"`
}

func (r SkillCopyCreated) MarshalJSON() ([]byte, error) {
	return encodeVariant("SkillCopyResult", "created", skillCopyCreatedWire(r))
}

func (r SkillCopyFailed) MarshalJSON() ([]byte, error) {
	return encodeVariant("SkillCopyResult", "failed", skillCopyFailedWire(r))
}

func (r UnknownSkillCopyResult) MarshalJSON() ([]byte, error) { return r.Raw, nil }

var skillCopyResultVariants = map[string]func([]byte) (SkillCopyResult, error){
	"created": variant(func(w skillCopyCreatedWire) SkillCopyResult { return SkillCopyCreated(w) }),
	"failed":  variant(func(w skillCopyFailedWire) SkillCopyResult { return SkillCopyFailed(w) }),
}

// decodeSkillCopyResults decodes the copySkill body, a list of SkillCopyResult.
func decodeSkillCopyResults(b *body, err error) ([]SkillCopyResult, error) {
	if err != nil {
		return nil, err
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(b.data, &raws); err != nil {
		return nil, b.decodeError(err)
	}
	out := make([]SkillCopyResult, len(raws))
	for i, raw := range raws {
		r, err := decodeUnion("SkillCopyResult", raw, skillCopyResultVariants, func(tag string, raw json.RawMessage) SkillCopyResult {
			return UnknownSkillCopyResult{Status: tag, Raw: raw}
		})
		if err != nil {
			return nil, b.decodeError(err)
		}
		out[i] = r
	}
	return out, nil
}

// checkFlat applies the required-key check of a flat union: a known tag needs its keys.
func checkFlat(name string, b []byte, tags ...string) error {
	known := map[string]func([]byte) (struct{}, error){}
	for _, t := range tags {
		known[t] = func([]byte) (struct{}, error) { return struct{}{}, nil }
	}
	_, err := decodeUnion(name, b, known, func(string, json.RawMessage) struct{} { return struct{}{} })
	return err
}

func (r *MCPConnectionReconnectResult) UnmarshalJSON(b []byte) error {
	type plain MCPConnectionReconnectResult
	if err := checkFlat("MCPConnectionReconnectResult", b, "connected", "needs_auth"); err != nil {
		return err
	}
	return json.Unmarshal(b, (*plain)(r))
}

type mcpConnectionTestWire struct {
	OK        bool                    `json:"ok"`
	Server    *MCPServerInfo          `json:"server,omitzero"`
	ToolNames []string                `json:"toolNames,omitzero"`
	LatencyMs *int64                  `json:"latencyMs,omitzero"`
	ToolCount *int64                  `json:"toolCount,omitzero"`
	Error     *MCPConnectionTestError `json:"error,omitzero"`
}

func (t MCPConnectionTest) MarshalJSON() ([]byte, error) {
	w := mcpConnectionTestWire{OK: t.OK, Server: t.Server, ToolNames: t.ToolNames, Error: t.Error}
	if t.OK {
		w.LatencyMs, w.ToolCount = new(t.Latency.Milliseconds()), new(t.ToolCount)
	}
	return json.Marshal(w)
}

func (t *MCPConnectionTest) UnmarshalJSON(b []byte) error {
	if err := checkFlat("MCPConnectionTest", b, "true", "false"); err != nil {
		return err
	}
	var w mcpConnectionTestWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*t = MCPConnectionTest{OK: w.OK, Server: w.Server, ToolNames: w.ToolNames, Error: w.Error}
	if w.LatencyMs != nil {
		t.Latency = time.Duration(*w.LatencyMs) * time.Millisecond
	}
	if w.ToolCount != nil {
		t.ToolCount = *w.ToolCount
	}
	return nil
}

// Host types decode their union field through a shadowing RawMessage field, so fields a
// worker adds to the host are decoded without any change here.

func (t *Task) UnmarshalJSON(b []byte) error {
	type plain Task
	var w struct {
		*plain
		Schedule json.RawMessage `json:"schedule"`
	}
	w.plain = (*plain)(t)
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var err error
	t.Schedule, err = decodeSchedule(w.Schedule)
	return err
}

func (t *TaskListItem) UnmarshalJSON(b []byte) error {
	type plain TaskListItem
	var w struct {
		*plain
		Schedule json.RawMessage `json:"schedule"`
	}
	w.plain = (*plain)(t)
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var err error
	t.Schedule, err = decodeSchedule(w.Schedule)
	return err
}

func (ws *Workspace) UnmarshalJSON(b []byte) error {
	type plain Workspace
	var w struct {
		*plain
		NetworkPolicy json.RawMessage `json:"networkPolicy"`
	}
	w.plain = (*plain)(ws)
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var err error
	ws.NetworkPolicy, err = decodeNetworkPolicy(w.NetworkPolicy)
	return err
}

func (c *ChatConnection) UnmarshalJSON(b []byte) error {
	type plain ChatConnection
	var w struct {
		*plain
		Configuration json.RawMessage `json:"configuration"`
	}
	w.plain = (*plain)(c)
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var err error
	c.Configuration, err = decodeChatConfiguration(w.Configuration)
	return err
}

func (a *ToolApproval) UnmarshalJSON(b []byte) error {
	type plain ToolApproval
	var w struct {
		*plain
		Tool json.RawMessage `json:"tool"`
	}
	w.plain = (*plain)(a)
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var err error
	a.Tool, err = decodeToolReference(w.Tool)
	return err
}

func (o *ApprovalOverride) UnmarshalJSON(b []byte) error {
	type plain ApprovalOverride
	var w struct {
		*plain
		Tool json.RawMessage `json:"tool"`
	}
	w.plain = (*plain)(o)
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var err error
	o.Tool, err = decodeToolReference(w.Tool)
	return err
}
