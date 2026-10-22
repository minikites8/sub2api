package service

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

// Context extraction for the Jev classifier (state-v3).
//
// The extractor makes one linear pass over the request's conversation array
// and keeps only fixed-size bookkeeping (counters, a ring of the most recent
// items, the latest user turns). Items are referenced, never copied, until
// the final selection; only selected items are unescaped, capped, redacted and
// copied. This replaces the v1/v2 approach that read the *first* 512 items
// and therefore lost the latest user message in long conversations.

// Context strategies.
const (
	// GPTAutoContextTaskAware prioritises the latest user instruction, the
	// request a short follow-up continues, current agent activity, the
	// original task and then recent conversation.
	GPTAutoContextTaskAware = "task_aware"
	// GPTAutoContextRecentTurns is the v2 behaviour: latest user message plus
	// the N preceding turns, without activity summaries.
	GPTAutoContextRecentTurns = "recent_turns"
)

// GPTAutoContextStrategies lists the supported strategies.
func GPTAutoContextStrategies() []string {
	return []string{GPTAutoContextTaskAware, GPTAutoContextRecentTurns}
}

// Bounds for the extractor. They cap memory and CPU independently of the
// request: the pass over the body is linear, and everything kept is fixed size.
const (
	// gptAutoMaxSnapshotBodyBytes: larger sampled requests are not
	// classified (counted as skipped_too_large) so the synchronous pass on the
	// request goroutine stays bounded.
	gptAutoMaxSnapshotBodyBytes = 16 << 20
	// gptAutoTailItems is how many of the most recent items are retained for
	// selection (by reference into the body).
	gptAutoTailItems = 64
	// gptAutoInspectBytes bounds per-item inspection (error detection,
	// harness/compaction markers) to a prefix/suffix of the item.
	gptAutoInspectBytes = 4096
	// gptAutoEventChars caps one agent-activity summary.
	gptAutoEventChars = 240
	// gptAutoOKResultChars caps the excerpt of a successful tool result.
	gptAutoOKResultChars = 80
	// gptAutoMaxToolNames caps the distinct tool names reported.
	gptAutoMaxToolNames = 12
	// gptAutoFollowUpMaxRunes: longer user messages are never follow-ups.
	gptAutoFollowUpMaxRunes = 80

	gptAutoDefaultAgentSteps = 8
	gptAutoMaxAgentSteps     = 30
)

// Context limitations reported to Jev and stored on the decision. They state
// what was not visible; nothing is fabricated to fill the gap.
const (
	GPTAutoLimitHistoryByReference = "history_by_reference"    // previous_response_id / conversation
	GPTAutoLimitItemReferences     = "item_references"         // item_reference inputs
	GPTAutoLimitHistoryCompacted   = "history_compacted"       // compaction items or summaries
	GPTAutoLimitNoUserMessage      = "no_user_message"         // no user text in the request
	GPTAutoLimitLatestUserTrimmed  = "latest_user_truncated"   // head/tail trimmed
	GPTAutoLimitContextOmitted     = "earlier_context_omitted" // turns/steps not forwarded
	GPTAutoLimitToolOutputs        = "tool_outputs_summarized"
	GPTAutoLimitImagesNotForwarded = "images_not_forwarded"
	GPTAutoLimitRequestTooLarge    = "request_too_large"
)

// GPTAutoContextOptions configures extraction.
type GPTAutoContextOptions struct {
	MaxChars     int    `json:"max_chars"`
	ContextTurns int    `json:"context_turns"`
	Strategy     string `json:"strategy"`
	AgentSteps   int    `json:"agent_steps"`
}

// ContextOptions returns the extraction options configured for this config.
func (c *GPTAutoShadowConfig) ContextOptions() GPTAutoContextOptions {
	return GPTAutoContextOptions{
		MaxChars:     c.ClassificationMaxContextChars,
		ContextTurns: c.ClassificationContextMessages,
		Strategy:     c.ClassificationContextStrategy,
		AgentSteps:   c.ClassificationAgentSteps,
	}
}

func (o GPTAutoContextOptions) normalized() GPTAutoContextOptions {
	if o.MaxChars <= 0 {
		o.MaxChars = gptAutoDefaultContextChars
	}
	o.ContextTurns = clampInt(o.ContextTurns, 0, gptAutoMaxContextMessages)
	if o.Strategy != GPTAutoContextRecentTurns {
		o.Strategy = GPTAutoContextTaskAware
	}
	o.AgentSteps = clampIntDefault(o.AgentSteps, 1, gptAutoMaxAgentSteps, gptAutoDefaultAgentSteps)
	return o
}

// Latest user message kinds.
const (
	GPTAutoUserKindInstruction = "instruction"
	GPTAutoUserKindFollowUp    = "follow_up"
	GPTAutoUserKindNone        = "none"
)

// GPTAutoSystemContext describes protected system/developer instructions
// without their text: whether they exist, a coarse size and what kind of
// client they suggest. Raw privileged instructions are never forwarded.
type GPTAutoSystemContext struct {
	Present bool   `json:"present"`
	Size    string `json:"size,omitempty"` // short | medium | long
	Kind    string `json:"kind,omitempty"` // agent_harness | assistant_persona | unspecified
	// HarnessMessages counts user-role messages injected by the client
	// (environment context, repository instruction files); they are treated
	// like system context, not as the user's request.
	HarnessMessages int `json:"harness_messages,omitempty"`
}

// GPTAutoTaskContext carries the task a short follow-up refers to and the
// conversation's original request, each bounded and redacted.
type GPTAutoTaskContext struct {
	ContinuesRequest string `json:"continues_request,omitempty"`
	OriginalRequest  string `json:"original_request,omitempty"`
	// HistorySummary is a client-written summary that replaced compacted
	// history (bounded). It is the client's text, not one the gateway wrote.
	HistorySummary string `json:"history_summary,omitempty"`
}

// GPTAutoAgentEvent is one bounded, sanitized step of an agent loop.
type GPTAutoAgentEvent struct {
	Kind    string `json:"kind"` // tool_call | tool_result | assistant_note
	Tool    string `json:"tool,omitempty"`
	Status  string `json:"status,omitempty"` // error | ok | unknown (tool_result)
	Summary string `json:"summary,omitempty"`
}

// GPTAutoAgentActivity summarises the agent loop relevant to the latest user
// message. Complete tool outputs are never forwarded.
type GPTAutoAgentActivity struct {
	// Scope is "since_latest_user_request", or "before_follow_up" when the
	// latest message is a follow-up sent right after the agent stopped.
	Scope            string              `json:"scope"`
	Steps            int                 `json:"steps"`
	ToolCalls        int                 `json:"tool_calls"`
	ToolResults      int                 `json:"tool_results"`
	FailedResults    int                 `json:"failed_results"`
	TrailingFailures int                 `json:"trailing_failures"`
	ToolsUsed        []string            `json:"tools_used,omitempty"`
	Recent           []GPTAutoAgentEvent `json:"recent,omitempty"`
	OmittedSteps     int                 `json:"omitted_steps,omitempty"`
}

// GPTAutoContextInfo is extraction metadata: counts, flags and character
// budgets only, never content. It is stored with each decision.
type GPTAutoContextInfo struct {
	StateVersion          string         `json:"state_version"`
	Strategy              string         `json:"strategy"`
	ItemsTotal            int            `json:"items_total"`
	ScannedBytes          int            `json:"scanned_bytes"`
	LatestUserIndex       int            `json:"latest_user_index"`
	LatestUserKind        string         `json:"latest_user_kind"`
	LatestUserTruncated   bool           `json:"latest_user_truncated"`
	FollowUpAnchorIndex   int            `json:"follow_up_anchor_index"`
	OriginalRequestIndex  int            `json:"original_request_index"`
	ContextTurnsAvailable int            `json:"context_turns_available"`
	ContextTurnsIncluded  int            `json:"context_turns_included"`
	AgentEventsIncluded   int            `json:"agent_events_included"`
	AgentEventsOmitted    int            `json:"agent_events_omitted"`
	BudgetChars           int            `json:"budget_chars"`
	UsedChars             int            `json:"used_chars"`
	SectionChars          map[string]int `json:"section_chars,omitempty"`
	Limitations           []string       `json:"limitations,omitempty"`
}

// gptAutoFollowUpPattern matches messages that continue earlier work rather
// than state a task: "continue", "fix it", "try again", "still failing", ...
var gptAutoFollowUpPattern = regexp.MustCompile(`(?i)^\s*(?:ok(?:ay)?[,.!]?\s*|yes[,.!]?\s*|please[,.!]?\s*|now[,.!]?\s*)*` +
	`(?:continue|go on|go ahead|keep going|carry on|proceed|resume|next|do it|do that|try again|retry|again|` +
	`fix (?:it|this|that|them)|fix the (?:error|errors|bug|tests?|build)|same (?:error|issue|problem)|still (?:failing|broken|not working|fails)|` +
	`it (?:still )?(?:fails|failed|doesn'?t work|does not work)|that didn'?t work|didn'?t work|not working|` +
	`yes|yep|ok|okay|sure|lgtm|sounds good|go|run it|apply it|undo that|revert that|继续|接着|继续吧|再试|再试一次|重试|修复|修一下|还是不行|还是报错|好的|可以|开始)` +
	`(?:\s+(?:please|pls|now|then|again|with that|from there))*\s*[.!。！?？]*\s*$`)

// gptAutoAnaphoraPattern catches short messages that only make sense with
// earlier context ("make it faster", "do the same for the other file").
var gptAutoAnaphoraPattern = regexp.MustCompile(`(?i)\b(?:it|this|that|these|those|the same|above|previous|again|as before)\b|上面|这个|那个|同样`)

// gptAutoIsFollowUp reports whether a user message continues earlier work.
// Only short messages qualify, and only when earlier turns exist.
func gptAutoIsFollowUp(text string, hasEarlierTurns bool) bool {
	text = strings.TrimSpace(text)
	if !hasEarlierTurns || text == "" || utf8.RuneCountInString(text) > gptAutoFollowUpMaxRunes {
		return false
	}
	if gptAutoFollowUpPattern.MatchString(text) {
		return true
	}
	return utf8.RuneCountInString(text) <= 40 && gptAutoAnaphoraPattern.MatchString(text)
}

// Harness markers: user-role messages that clients inject automatically.
var gptAutoHarnessPrefixes = []string{
	"<environment_context>", "<user_instructions>", "<system-reminder>", "<system_reminder>",
	"# agents.md instructions", "<permissions instructions>", "<turn_aborted>", "<user_shell_command>",
	"<command-name>", "<local-command-stdout>", "caveat: the messages below were generated",
}

// Compaction markers: summaries that stand in for history the client dropped.
var gptAutoCompactionPrefixes = []string{
	"another language model started to solve this problem",
	"this session is being continued from a previous conversation",
	"summary of the conversation so far",
	"<conversation_summary>", "<summary>", "[conversation summary]", "previous conversation summary",
}

func gptAutoHasPrefixFold(text string, prefixes []string) bool {
	head := strings.ToLower(strings.TrimSpace(text))
	if len(head) > 256 {
		head = head[:256]
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(head, prefix) {
			return true
		}
	}
	return false
}

// Item kinds.
const (
	gptAutoKindMessage = iota
	gptAutoKindToolCall
	gptAutoKindToolResult
	gptAutoKindReasoning
	gptAutoKindReference
	gptAutoKindCompaction
	gptAutoKindOther
)

// gptAutoFields are the item members the scan needs, read in one pass over
// the item's keys (repeated item.Get calls would rescan large items).
type gptAutoFields struct {
	typ, role, callID, toolCallID, name string
	content, output, arguments, input   gjson.Result
	toolCalls, action, summary          gjson.Result
}

func gptAutoParseFields(item gjson.Result) gptAutoFields {
	var f gptAutoFields
	gptAutoObjectMembers(item.Raw, func(key string, value gjson.Result) bool {
		switch key {
		case "type":
			f.typ = gptAutoRawString(value)
		case "role":
			f.role = gptAutoRawString(value)
		case "call_id":
			f.callID = gptAutoRawString(value)
		case "tool_call_id":
			f.toolCallID = gptAutoRawString(value)
		case "name":
			f.name = gptAutoRawString(value)
		case "content":
			f.content = value
		case "output":
			f.output = value
		case "arguments":
			f.arguments = value
		case "input":
			f.input = value
		case "tool_calls":
			f.toolCalls = value
		case "action":
			f.action = value
		case "summary":
			f.summary = value
		}
		return true
	})
	return f
}

// gptAutoRawString returns a short string value. Identifiers rarely contain
// escapes; when they do, the (small) value is decoded.
func gptAutoRawString(value gjson.Result) string {
	if value.Type != gjson.String || len(value.Raw) < 2 {
		return ""
	}
	inner := value.Raw[1 : len(value.Raw)-1]
	if !strings.Contains(inner, `\`) {
		return inner
	}
	if len(inner) > 256 {
		inner = inner[:256]
	}
	return gptAutoLenientUnescape(inner)
}

// gptAutoObjectMembers iterates the top-level members of a JSON object and
// hands each value over as a raw, undecoded gjson.Result. Unlike
// gjson.ForEach/Get it never unescapes string values, so a multi-megabyte
// tool output costs one byte scan and no allocation.
func gptAutoObjectMembers(raw string, fn func(key string, value gjson.Result) bool) {
	i := gptAutoSkipSpace(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return
	}
	i++
	for {
		i = gptAutoSkipSpace(raw, i)
		if i >= len(raw) || raw[i] == '}' || raw[i] != '"' {
			return
		}
		keyEnd := gptAutoStringEnd(raw, i)
		if keyEnd < 0 {
			return
		}
		key := raw[i+1 : keyEnd-1]
		i = gptAutoSkipSpace(raw, keyEnd)
		if i >= len(raw) || raw[i] != ':' {
			return
		}
		i = gptAutoSkipSpace(raw, i+1)
		end := gptAutoValueEnd(raw, i)
		if end < 0 {
			return
		}
		value := gjson.Result{Raw: raw[i:end]}
		switch raw[i] {
		case '"':
			value.Type = gjson.String
		case '{', '[':
			value.Type = gjson.JSON
		case 't':
			value.Type = gjson.True
		case 'f':
			value.Type = gjson.False
		case 'n':
			value.Type = gjson.Null
		default:
			value.Type = gjson.Number
		}
		if !fn(key, value) {
			return
		}
		i = gptAutoSkipSpace(raw, end)
		if i >= len(raw) || raw[i] != ',' {
			return
		}
		i++
	}
}

func gptAutoSkipSpace(raw string, i int) int {
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\r' || raw[i] == '\t') {
		i++
	}
	return i
}

// gptAutoStringEnd returns the offset just past the string literal at i.
func gptAutoStringEnd(raw string, i int) int {
	for j := i + 1; j < len(raw); j++ {
		switch raw[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return -1
}

// gptAutoValueEnd returns the offset just past the JSON value at i.
func gptAutoValueEnd(raw string, i int) int {
	if i >= len(raw) {
		return -1
	}
	switch raw[i] {
	case '"':
		return gptAutoStringEnd(raw, i)
	case '{', '[':
		depth := 0
		for j := i; j < len(raw); j++ {
			switch raw[j] {
			case '"':
				end := gptAutoStringEnd(raw, j)
				if end < 0 {
					return -1
				}
				j = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return j + 1
				}
			}
		}
		return -1
	default:
		j := i
		for j < len(raw) && raw[j] != ',' && raw[j] != '}' && raw[j] != ']' && raw[j] != ' ' && raw[j] != '\n' && raw[j] != '\r' && raw[j] != '\t' {
			j++
		}
		return j
	}
}

// chars approximates the item's prompt characters from its payload fields
// rather than its JSON envelope.
func (f gptAutoFields) chars() int {
	return gptAutoValueChars(f.content) + gptAutoValueChars(f.output) + gptAutoValueChars(f.arguments) +
		gptAutoValueChars(f.input) + gptAutoValueChars(f.toolCalls) + gptAutoValueChars(f.action) + gptAutoValueChars(f.summary)
}

// gptAutoFieldsKind classifies one Chat Completions message or Responses
// input item and returns its normalised role.
func gptAutoFieldsKind(f gptAutoFields) (int, string) {
	switch f.typ {
	case "function_call", "custom_tool_call", "local_shell_call", "computer_call", "mcp_call",
		"web_search_call", "file_search_call", "code_interpreter_call", "image_generation_call":
		return gptAutoKindToolCall, "assistant"
	case "function_call_output", "custom_tool_call_output", "local_shell_call_output", "computer_call_output", "mcp_call_output":
		return gptAutoKindToolResult, "tool"
	case "reasoning":
		return gptAutoKindReasoning, "assistant"
	case "item_reference":
		return gptAutoKindReference, ""
	case "compaction", "compaction_summary", "compacted", "context_compaction":
		return gptAutoKindCompaction, ""
	}
	if f.role == "" {
		return gptAutoKindOther, ""
	}
	normalized := gptAutoNormalizeRole(f.role)
	if normalized == "tool" {
		return gptAutoKindToolResult, normalized
	}
	return gptAutoKindMessage, normalized
}

// gptAutoLenientUnescape decodes a window cut out of a JSON string literal.
// A window may start or end inside an escape sequence; such fragments are
// dropped rather than failing the whole window.
func gptAutoLenientUnescape(raw string) string {
	if !strings.Contains(raw, `\`) {
		return raw
	}
	var out strings.Builder
	out.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' {
			_ = out.WriteByte(c)
			continue
		}
		if i+1 >= len(raw) {
			break
		}
		i++
		switch raw[i] {
		case 'n':
			_ = out.WriteByte('\n')
		case 't':
			_ = out.WriteByte('\t')
		case 'r':
			_ = out.WriteByte('\r')
		case 'b', 'f':
			_ = out.WriteByte(' ')
		case '"', '\\', '/':
			_ = out.WriteByte(raw[i])
		case 'u':
			if i+4 < len(raw) {
				if code, err := strconv.ParseUint(raw[i+1:i+5], 16, 32); err == nil {
					_, _ = out.WriteRune(rune(code))
				}
				i += 4
			} else {
				i = len(raw)
			}
		}
	}
	return strings.ToValidUTF8(out.String(), "")
}

// gptAutoStringWindow returns the text of a JSON string value bounded to about
// maxRunes, keeping head and tail, without unescaping the whole value.
func gptAutoStringWindow(value gjson.Result, maxRunes int) string {
	if value.Type != gjson.String || maxRunes <= 0 {
		return ""
	}
	raw := value.Raw
	if len(raw) >= 2 {
		raw = raw[1 : len(raw)-1]
	}
	limit := maxRunes * 6
	if len(raw) <= limit*2 {
		return trimRunesKeepTail(gptAutoLenientUnescape(raw), maxRunes)
	}
	head := gptAutoLenientUnescape(raw[:limit])
	tail := gptAutoLenientUnescape(raw[len(raw)-limit:])
	return trimRunesKeepTail(head+" … "+tail, maxRunes)
}

// gptAutoContentText extracts bounded text from a "content" or "output"
// value (string or array of parts) and reports whether it carried images.
func gptAutoContentText(content gjson.Result, maxRunes int) (string, bool) {
	if content.Type == gjson.String {
		return gptAutoStringWindow(content, maxRunes), false
	}
	if !content.IsArray() {
		return "", false
	}
	// The first and last 32 text parts are kept; the middle of a very long
	// part list is dropped like the middle of a long string.
	var head, tail []string
	hasImages := false
	count := 0
	content.ForEach(func(_, part gjson.Result) bool {
		count++
		var typ string
		var text gjson.Result
		gptAutoObjectMembers(part.Raw, func(key string, value gjson.Result) bool {
			switch key {
			case "type":
				typ = gptAutoRawString(value)
			case "text":
				text = value
			}
			return true
		})
		switch typ {
		case "image_url", "input_image", "image", "output_image":
			hasImages = true
		default:
			if text.Type != gjson.String {
				break
			}
			window := gptAutoStringWindow(text, maxRunes)
			if len(head) < 32 {
				head = append(head, window)
			} else {
				tail = append(tail, window)
				if len(tail) > 32 {
					tail = tail[1:]
				}
			}
		}
		return count < 4096
	})
	parts := append(head, tail...)
	return trimRunesKeepTail(strings.Join(parts, "\n"), maxRunes), hasImages
}

// gptAutoValueChars approximates the characters a value contributes to the
// prompt without decoding it.
func gptAutoValueChars(value gjson.Result) int {
	if value.Type == gjson.String && len(value.Raw) >= 2 {
		return utf8.RuneCountInString(value.Raw) - 2
	}
	return utf8.RuneCountInString(value.Raw)
}

// gptAutoScanItem references one conversation item inside the request body.
type gptAutoScanItem struct {
	index    int
	kind     int
	role     string
	item     gjson.Result
	followUp bool
}

// gptAutoSegment counts agent activity between two user turns.
type gptAutoSegment struct {
	start            int
	steps            int
	toolCalls        int
	toolResults      int
	failed           int
	trailingFailures int
	tools            []string
}

func (s *gptAutoSegment) addTool(name string) {
	name = strings.TrimSpace(name)
	if name == "" || len(s.tools) >= gptAutoMaxToolNames || containsString(s.tools, name) {
		return
	}
	s.tools = append(s.tools, trimRunes(name, 64))
}

// gptAutoRing keeps the last n items in arrival order.
type gptAutoRing struct {
	items []gptAutoScanItem
	next  int
	full  bool
}

func newGPTAutoRing(n int) gptAutoRing { return gptAutoRing{items: make([]gptAutoScanItem, n)} }

func (r *gptAutoRing) push(item gptAutoScanItem) {
	r.items[r.next] = item
	r.next = (r.next + 1) % len(r.items)
	if r.next == 0 {
		r.full = true
	}
}

func (r *gptAutoRing) ordered() []gptAutoScanItem {
	return r.orderedInto(nil)
}

// orderedInto appends the items in arrival order to dst (reusing its
// capacity, so snapshotting the ring at every user turn does not allocate).
func (r *gptAutoRing) orderedInto(dst []gptAutoScanItem) []gptAutoScanItem {
	if !r.full {
		return append(dst, r.items[:r.next]...)
	}
	return append(append(dst, r.items[r.next:]...), r.items[:r.next]...)
}

// gptAutoScan is the fixed-size result of the linear pass.
type gptAutoScan struct {
	total             int
	chars             int
	conversationItems int
	hasImages         bool
	historyByRef      bool
	references        bool
	compaction        bool
	compactionSummary *gptAutoScanItem
	system            GPTAutoSystemContext
	systemChars       int
	firstRequest      *gptAutoScanItem
	latestUser        *gptAutoScanItem
	anchor            *gptAutoScanItem
	current           gptAutoSegment
	previous          gptAutoSegment
	tail              gptAutoRing
	beforeLatest      []gptAutoScanItem
	toolNames         map[string]string
}

var gptAutoAgentHarnessHint = regexp.MustCompile(`(?i)\b(?:tools?|shell|terminal|sandbox|repositor(?:y|ies)|codebase|apply_patch|function call|agent|workspace|command line)\b`)

func (s *gptAutoScan) observeSystem(content gjson.Result) {
	s.system.Present = true
	s.systemChars += gptAutoValueChars(content)
	if s.system.Kind != "agent_harness" {
		raw := content.Raw
		if len(raw) > gptAutoInspectBytes {
			raw = raw[:gptAutoInspectBytes]
		}
		if gptAutoAgentHarnessHint.MatchString(raw) {
			s.system.Kind = "agent_harness"
		} else {
			s.system.Kind = "assistant_persona"
		}
	}
}

func (s *gptAutoScan) rememberToolName(callID, name string) {
	if callID == "" || name == "" {
		return
	}
	if len(s.toolNames) >= 256 {
		s.toolNames = map[string]string{}
	}
	s.toolNames[callID] = name
}

// gptAutoToolCallName returns the tool name of a Responses tool-call item.
func gptAutoToolCallName(item gjson.Result) string {
	return gptAutoFieldsToolName(gptAutoParseFields(item))
}

func gptAutoFieldsToolName(f gptAutoFields) string {
	switch f.typ {
	case "local_shell_call":
		return "local_shell"
	case "web_search_call":
		return "web_search"
	case "file_search_call":
		return "file_search"
	case "code_interpreter_call":
		return "code_interpreter"
	case "computer_call":
		return "computer"
	case "image_generation_call":
		return "image_generation"
	}
	return f.name
}

func (s *gptAutoScan) observe(index int, item gjson.Result) {
	s.total++
	fields := gptAutoParseFields(item)
	s.chars += fields.chars()
	kind, role := gptAutoFieldsKind(fields)
	entry := gptAutoScanItem{index: index, kind: kind, role: role, item: item}
	switch kind {
	case gptAutoKindReference:
		s.references = true
		return
	case gptAutoKindCompaction:
		s.compaction = true
		s.conversationItems++
		return
	case gptAutoKindReasoning:
		s.current.steps++
		return
	case gptAutoKindOther:
		return
	case gptAutoKindToolCall:
		s.conversationItems++
		s.current.steps++
		s.current.toolCalls++
		name := gptAutoFieldsToolName(fields)
		s.current.addTool(name)
		s.rememberToolName(fields.callID, name)
		s.tail.push(entry)
		return
	case gptAutoKindToolResult:
		s.conversationItems++
		s.current.steps++
		s.current.toolResults++
		name := s.toolNames[fields.callID]
		if name == "" {
			name = s.toolNames[fields.toolCallID]
		}
		s.current.addTool(name)
		output := fields.output
		if !output.Exists() {
			output = fields.content
		}
		if gptAutoQuickFailed(output) {
			s.current.failed++
			s.current.trailingFailures++
		} else {
			s.current.trailingFailures = 0
		}
		s.tail.push(entry)
		return
	}

	// Messages.
	switch role {
	case "system":
		s.observeSystem(fields.content)
		return
	case "assistant":
		s.conversationItems++
		s.current.steps++
		fields.toolCalls.ForEach(func(_, call gjson.Result) bool {
			s.current.toolCalls++
			name := call.Get("function.name").String()
			s.current.addTool(name)
			s.rememberToolName(call.Get("id").String(), name)
			return true
		})
		s.tail.push(entry)
		return
	case "user":
	default:
		return
	}

	text, images := gptAutoContentText(fields.content, 200)
	if images {
		s.hasImages = true
	}
	if strings.TrimSpace(text) == "" && !images {
		return
	}
	if gptAutoHasPrefixFold(text, gptAutoHarnessPrefixes) {
		s.system.HarnessMessages++
		return
	}
	if gptAutoHasPrefixFold(text, gptAutoCompactionPrefixes) {
		s.compaction = true
		s.conversationItems++
		summary := entry
		s.compactionSummary = &summary
		return
	}
	entry.followUp = gptAutoIsFollowUp(text, s.conversationItems > 0 || s.historyByRef)
	s.conversationItems++
	if s.latestUser != nil && !s.latestUser.followUp {
		anchor := *s.latestUser
		s.anchor = &anchor
	}
	latest := entry
	s.latestUser = &latest
	if s.firstRequest == nil && !entry.followUp {
		first := entry
		s.firstRequest = &first
	}
	s.beforeLatest = s.tail.orderedInto(s.beforeLatest[:0])
	s.previous = s.current
	s.current = gptAutoSegment{start: index}
	s.tail.push(entry)
}

// gptAutoToolOutput returns the output value of a tool-result item.
func gptAutoToolOutput(item gjson.Result) gjson.Result {
	if output := item.Get("output"); output.Exists() {
		return output
	}
	return item.Get("content")
}

// gptAutoInspectWindow returns a decoded head and tail of a value for status
// detection and error excerpts. It never decodes more than a few KB.
func gptAutoInspectWindow(value gjson.Result) string {
	raw := value.Raw
	if value.Type == gjson.String && len(raw) >= 2 {
		raw = raw[1 : len(raw)-1]
	}
	if len(raw) <= gptAutoInspectBytes+1024 {
		return gptAutoLenientUnescape(raw)
	}
	return gptAutoLenientUnescape(raw[:1024]) + "\n…\n" + gptAutoLenientUnescape(raw[len(raw)-gptAutoInspectBytes:])
}

// gptAutoQuickFailed decides a tool result's status during the scan from raw
// (still JSON-escaped) windows: the first 512 bytes for an exit code and the
// last 2 KB for error markers. Nothing is decoded or allocated.
func gptAutoQuickFailed(value gjson.Result) bool {
	raw := value.Raw
	head, tail := raw, raw
	if len(head) > 512 {
		head = head[:512]
	}
	if len(tail) > 2048 {
		tail = tail[len(tail)-2048:]
	}
	if match := gptAutoExitCodePattern.FindStringSubmatch(head); match != nil {
		return match[1] != "0"
	}
	if match := gptAutoExitCodePattern.FindStringSubmatch(tail); match != nil {
		return match[1] != "0"
	}
	return gptAutoErrorLine.MatchString(tail)
}

var (
	gptAutoExitCodePattern = regexp.MustCompile(`(?i)(?:exit[ _]?code|exit status|"exit_code")"?\s*[:=]?\s*(-?\d+)`)
	gptAutoErrorLine       = regexp.MustCompile(`(?i)\b(?:error|exception|traceback|panic|fatal|failed|failure|fail:|segmentation fault|command not found|permission denied|no such file|cannot|could not|undefined|not found|timed out|refused)\b|--- FAIL|✗|❌`)
)

// gptAutoToolStatus classifies a tool result from its inspection window. An
// explicit exit code wins over keywords.
func gptAutoToolStatus(window string) string {
	if match := gptAutoExitCodePattern.FindStringSubmatch(window); match != nil {
		if match[1] == "0" {
			return "ok"
		}
		return "error"
	}
	if strings.TrimSpace(window) == "" {
		return "unknown"
	}
	if gptAutoErrorLine.MatchString(window) {
		return "error"
	}
	return "ok"
}

// gptAutoErrorExcerpt keeps the error block of a failed tool result: from the
// first error-looking line among the last 40 lines, a few lines onward (test
// failures put the detail on the line after the marker).
func gptAutoErrorExcerpt(window string, maxRunes int) string {
	lines := strings.Split(window, "\n")
	start := len(lines) - 40
	if start < 0 {
		start = 0
	}
	from := -1
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line != "" && !gptAutoOutputMetaLine.MatchString(line) && gptAutoErrorLine.MatchString(line) {
			from = i
			break
		}
	}
	if from < 0 {
		return gptAutoTailRunes(gptAutoCollapseSpace(window), maxRunes)
	}
	var picked []string
	for i := from; i < len(lines) && len(picked) < 6; i++ {
		if line := strings.TrimSpace(lines[i]); line != "" {
			picked = append(picked, line)
		}
	}
	return trimRunes(strings.Join(picked, " | "), maxRunes)
}

// gptAutoOutputMetaLine matches the metadata header agent clients prepend to
// tool output ("Exit code: 0", "Wall time: 1.2 seconds", "Output:").
var gptAutoOutputMetaLine = regexp.MustCompile(`(?i)^(?:exit code:|exit status:|wall time:|output:|total output lines:|process exited|chunk id:)`)

func gptAutoFirstLine(text string, maxRunes int) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" && !gptAutoOutputMetaLine.MatchString(line) {
			return trimRunes(line, maxRunes)
		}
	}
	return ""
}

func gptAutoCollapseSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// gptAutoTailRunes keeps the last n runes (progress notes end with the
// current state, not the preamble).
func gptAutoTailRunes(text string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= n {
		return text
	}
	return "…" + text[gptAutoTailOffset(text, n-1):]
}

// gptAutoSanitize redacts and bounds text for the state.
func gptAutoSanitize(text string, maxRunes int) string {
	return trimRunesKeepTail(redactGPTAutoText(strings.TrimSpace(text)), maxRunes)
}

// gptAutoToolArguments returns a short excerpt of what a tool was asked to do.
func gptAutoToolArguments(item gjson.Result, call gjson.Result) string {
	var value gjson.Result
	switch {
	case call.Exists():
		value = call.Get("function.arguments")
	case item.Get("type").String() == "local_shell_call":
		value = item.Get("action.command")
	case item.Get("arguments").Exists():
		value = item.Get("arguments")
	case item.Get("input").Exists():
		value = item.Get("input")
	case item.Get("action").Exists():
		value = item.Get("action")
	}
	if value.Type == gjson.String {
		return gptAutoCollapseSpace(gptAutoStringWindow(value, 400))
	}
	raw := value.Raw
	if len(raw) > 1600 {
		raw = raw[:1600]
	}
	return gptAutoCollapseSpace(gptAutoLenientUnescape(raw))
}

// gptAutoItemEvents turns one retained item into agent events.
func gptAutoItemEvents(entry gptAutoScanItem, toolNames map[string]string) []GPTAutoAgentEvent {
	item := entry.item
	switch entry.kind {
	case gptAutoKindToolCall:
		return []GPTAutoAgentEvent{{Kind: "tool_call", Tool: gptAutoToolCallName(item),
			Summary: gptAutoSanitize(gptAutoToolArguments(item, gjson.Result{}), gptAutoEventChars)}}
	case gptAutoKindToolResult:
		name := toolNames[item.Get("call_id").String()]
		if name == "" {
			name = toolNames[item.Get("tool_call_id").String()]
		}
		window := gptAutoInspectWindow(gptAutoToolOutput(item))
		event := GPTAutoAgentEvent{Kind: "tool_result", Tool: name, Status: gptAutoToolStatus(window)}
		switch event.Status {
		case "error":
			event.Summary = gptAutoSanitize(gptAutoErrorExcerpt(window, gptAutoEventChars), gptAutoEventChars)
		case "ok":
			event.Summary = gptAutoSanitize(gptAutoFirstLine(window, gptAutoOKResultChars), gptAutoOKResultChars)
		}
		return []GPTAutoAgentEvent{event}
	case gptAutoKindMessage:
		if entry.role != "assistant" {
			return nil
		}
		var events []GPTAutoAgentEvent
		if text, _ := gptAutoContentText(item.Get("content"), 1200); strings.TrimSpace(text) != "" {
			events = append(events, GPTAutoAgentEvent{Kind: "assistant_note",
				Summary: gptAutoSanitize(gptAutoTailRunes(gptAutoCollapseSpace(text), gptAutoEventChars), gptAutoEventChars)})
		}
		item.Get("tool_calls").ForEach(func(_, call gjson.Result) bool {
			events = append(events, GPTAutoAgentEvent{Kind: "tool_call", Tool: call.Get("function.name").String(),
				Summary: gptAutoSanitize(gptAutoToolArguments(item, call), gptAutoEventChars)})
			return len(events) < 16
		})
		return events
	}
	return nil
}

// gptAutoRecentEvents returns up to n most recent events from items.
func gptAutoRecentEvents(items []gptAutoScanItem, toolNames map[string]string, n int) []GPTAutoAgentEvent {
	var events []GPTAutoAgentEvent
	for i := len(items) - 1; i >= 0 && len(events) < n; i-- {
		itemEvents := gptAutoItemEvents(items[i], toolNames)
		events = append(itemEvents, events...)
	}
	if len(events) > n {
		events = events[len(events)-n:]
	}
	return events
}

func gptAutoEventsChars(events []GPTAutoAgentEvent) int {
	total := 0
	for _, event := range events {
		total += utf8.RuneCountInString(event.Summary) + utf8.RuneCountInString(event.Tool) + 24
	}
	return total
}

func gptAutoSizeBucket(chars int) string {
	switch {
	case chars < 2000:
		return "short"
	case chars < 20000:
		return "medium"
	default:
		return "long"
	}
}

func gptAutoAddLimitation(list []string, limitation string) []string {
	if containsString(list, limitation) {
		return list
	}
	return append(list, limitation)
}

// BuildGPTAutoSnapshot extracts a classification snapshot from an
// OpenAI-compatible request body. It reads the body and never modifies it,
// and the returned snapshot shares no memory with it.
//
// It runs on the request goroutine for sampled requests. Cost is one linear
// pass over the conversation array plus bounded work on a fixed number of
// selected items; bodies above gptAutoMaxSnapshotBodyBytes are not scanned.
func BuildGPTAutoSnapshot(endpoint string, body []byte, opts GPTAutoContextOptions) GPTAutoRequestSnapshot {
	opts = opts.normalized()
	// Zero-copy view: gjson.GetBytes would copy the whole conversation array.
	// The body is not modified during this call, and gptAutoDetach clones
	// every retained string before returning.
	view := openAIWSPayloadStringView(body)
	snapshot := GPTAutoRequestSnapshot{
		Endpoint:       endpoint,
		RequestedModel: gjson.Get(view, "model").String(),
		Stream:         gjson.Get(view, "stream").Bool(),
		LatestUserKind: GPTAutoUserKindNone,
	}
	snapshot.Context = GPTAutoContextInfo{StateVersion: GPTAutoClassifierStateVersion, Strategy: opts.Strategy, BudgetChars: opts.MaxChars,
		LatestUserIndex: -1, FollowUpAnchorIndex: -1, OriginalRequestIndex: -1, ScannedBytes: len(body)}
	if len(body) > gptAutoMaxSnapshotBodyBytes {
		snapshot.Limitations = []string{GPTAutoLimitRequestTooLarge}
		snapshot.Context.Limitations = snapshot.Limitations
		snapshot.Context.ScannedBytes = 0
		return snapshot
	}
	snapshot.RequestedEffort = strings.ToLower(strings.TrimSpace(gjson.Get(view, "reasoning.effort").String()))
	if snapshot.RequestedEffort == "" {
		snapshot.RequestedEffort = strings.ToLower(strings.TrimSpace(gjson.Get(view, "reasoning_effort").String()))
	}
	if tools := gjson.Get(view, "tools"); tools.IsArray() {
		snapshot.ToolCount = int(tools.Get("#").Int())
	}
	if functions := gjson.Get(view, "functions"); functions.IsArray() {
		snapshot.ToolCount += int(functions.Get("#").Int())
	}
	snapshot.HasTools = snapshot.ToolCount > 0

	scan := &gptAutoScan{tail: newGPTAutoRing(gptAutoTailItems), toolNames: map[string]string{}, current: gptAutoSegment{start: -1}}
	switch endpoint {
	case GPTAutoEndpointResponses:
		prev := strings.TrimSpace(gjson.Get(view, "previous_response_id").String())
		scan.historyByRef = prev != "" || gjson.Get(view, "conversation").Exists()
		if instructions := gjson.Get(view, "instructions"); instructions.Type == gjson.String && len(instructions.Raw) > 2 {
			scan.chars += gptAutoValueChars(instructions)
			scan.observeSystem(instructions)
		}
		input := gjson.Get(view, "input")
		if input.Type == gjson.String {
			// A plain-string input is one user message.
			scan.observe(0, gjson.Parse(`{"role":"user","content":`+input.Raw+`}`))
		} else if input.IsArray() {
			index := 0
			input.ForEach(func(_, item gjson.Result) bool {
				scan.observe(index, item)
				index++
				return true
			})
		}
	default:
		index := 0
		gjson.Get(view, "messages").ForEach(func(_, item gjson.Result) bool {
			scan.observe(index, item)
			index++
			return true
		})
	}
	snapshot.MessageCount = scan.total
	snapshot.EstimatedInputTokens = (scan.chars + gptAutoCharsPerToken - 1) / gptAutoCharsPerToken
	snapshot.HasImages = scan.hasImages
	gptAutoAssembleContext(&snapshot, scan, opts)
	gptAutoDetach(&snapshot)
	return snapshot
}

// gptAutoDetach clones every string the snapshot keeps so it shares no memory
// with the request body (substrings and unchanged regex results would).
func gptAutoDetach(snapshot *GPTAutoRequestSnapshot) {
	snapshot.RequestedModel = strings.Clone(snapshot.RequestedModel)
	snapshot.RequestedEffort = strings.Clone(snapshot.RequestedEffort)
	snapshot.LatestUser = strings.Clone(snapshot.LatestUser)
	for i := range snapshot.RecentContext {
		snapshot.RecentContext[i].Text = strings.Clone(snapshot.RecentContext[i].Text)
	}
	if task := snapshot.TaskContext; task != nil {
		task.ContinuesRequest = strings.Clone(task.ContinuesRequest)
		task.OriginalRequest = strings.Clone(task.OriginalRequest)
		task.HistorySummary = strings.Clone(task.HistorySummary)
	}
	if activity := snapshot.AgentActivity; activity != nil {
		tools := make([]string, len(activity.ToolsUsed))
		for i, name := range activity.ToolsUsed {
			tools[i] = strings.Clone(name)
		}
		activity.ToolsUsed = tools
		for i := range activity.Recent {
			activity.Recent[i].Tool = strings.Clone(activity.Recent[i].Tool)
			activity.Recent[i].Summary = strings.Clone(activity.Recent[i].Summary)
		}
	}
}

// gptAutoAssembleContext selects, bounds and redacts what Jev may see, in
// priority order, from the fixed-size scan result.
func gptAutoAssembleContext(snapshot *GPTAutoRequestSnapshot, scan *gptAutoScan, opts GPTAutoContextOptions) {
	info := &snapshot.Context
	info.ItemsTotal = scan.total
	sections := map[string]int{}
	remaining := opts.MaxChars
	capture := opts.MaxChars * gptAutoTurnCaptureFactor
	taskAware := opts.Strategy == GPTAutoContextTaskAware
	take := func(section, text string, limit int) string {
		limit = minInt(limit, remaining)
		if limit <= 0 {
			return ""
		}
		out := gptAutoSanitize(text, limit)
		used := utf8.RuneCountInString(out)
		remaining -= used
		sections[section] += used
		return out
	}
	var limitations []string
	if scan.historyByRef {
		limitations = append(limitations, GPTAutoLimitHistoryByReference)
	}
	if scan.references {
		limitations = append(limitations, GPTAutoLimitItemReferences)
	}
	if scan.compaction {
		limitations = append(limitations, GPTAutoLimitHistoryCompacted)
	}
	if scan.hasImages {
		limitations = append(limitations, GPTAutoLimitImagesNotForwarded)
	}

	snapshot.SystemContext = scan.system
	if scan.system.Present {
		snapshot.SystemContext.Size = gptAutoSizeBucket(scan.systemChars)
	}
	if (scan.system.Present || scan.system.HarnessMessages > 0) && snapshot.SystemContext.Kind == "" {
		snapshot.SystemContext.Kind = "unspecified"
	}
	snapshot.HasSystemPrompt = scan.system.Present || scan.system.HarnessMessages > 0
	visible := 0

	// 1. Latest user instruction.
	latest := scan.latestUser
	if latest != nil {
		info.LatestUserIndex = latest.index
		snapshot.LatestUserKind = GPTAutoUserKindInstruction
		share := opts.MaxChars / 2
		if latest.followUp {
			snapshot.LatestUserKind = GPTAutoUserKindFollowUp
			share = opts.MaxChars / 4
		}
		if !taskAware {
			share = opts.MaxChars * 3 / 4
		}
		text, _ := gptAutoContentText(latest.item.Get("content"), capture)
		snapshot.LatestUser = take("latest_user_request", text, share)
		if utf8.RuneCountInString(snapshot.LatestUser) < utf8.RuneCountInString(strings.TrimSpace(text)) ||
			gptAutoValueChars(latest.item.Get("content")) > capture*6 {
			info.LatestUserTruncated = true
			limitations = append(limitations, GPTAutoLimitLatestUserTrimmed)
		}
		visible++
	} else {
		limitations = append(limitations, GPTAutoLimitNoUserMessage)
	}
	info.LatestUserKind = snapshot.LatestUserKind

	if taskAware {
		task := &GPTAutoTaskContext{}
		// 2. The request a short follow-up continues.
		if latest != nil && latest.followUp && scan.anchor != nil {
			text, _ := gptAutoContentText(scan.anchor.item.Get("content"), capture)
			task.ContinuesRequest = take("continues_request", text, opts.MaxChars/4)
			info.FollowUpAnchorIndex = scan.anchor.index
			visible++
		}
		// 3. Current agent activity.
		snapshot.AgentActivity = gptAutoActivity(scan, opts, remaining, opts.MaxChars/4)
		if activity := snapshot.AgentActivity; activity != nil {
			used := gptAutoEventsChars(activity.Recent)
			remaining -= used
			sections["agent_activity"] = used
			info.AgentEventsIncluded = len(activity.Recent)
			info.AgentEventsOmitted = activity.OmittedSteps
			visible += len(activity.Recent)
			if activity.ToolResults > 0 {
				limitations = append(limitations, GPTAutoLimitToolOutputs)
			}
		}
		// 4. Original task and any client-provided history summary.
		if first := scan.firstRequest; first != nil && (latest == nil || first.index != latest.index) &&
			(scan.anchor == nil || info.FollowUpAnchorIndex != first.index) {
			text, _ := gptAutoContentText(first.item.Get("content"), capture)
			task.OriginalRequest = take("original_request", text, opts.MaxChars*15/100)
			info.OriginalRequestIndex = first.index
			visible++
		}
		if summary := scan.compactionSummary; summary != nil {
			text, _ := gptAutoContentText(summary.item.Get("content"), capture)
			task.HistorySummary = take("history_summary", text, opts.MaxChars*15/100)
			visible++
		}
		if *task != (GPTAutoTaskContext{}) {
			snapshot.TaskContext = task
		}
	}

	// 5. Recent conversation before the latest user message.
	candidates := scan.tail.ordered()
	if latest != nil {
		candidates = scan.beforeLatest
	}
	var turns []gptAutoScanItem
	for _, entry := range candidates {
		if latest != nil && entry.index >= latest.index {
			continue
		}
		if taskAware {
			if entry.kind != gptAutoKindMessage || (entry.role != "user" && entry.role != "assistant") ||
				entry.index == info.FollowUpAnchorIndex || entry.index == info.OriginalRequestIndex {
				continue
			}
		} else if entry.kind != gptAutoKindMessage && entry.kind != gptAutoKindToolResult {
			continue
		}
		turns = append(turns, entry)
	}
	info.ContextTurnsAvailable = len(turns)
	if len(turns) > opts.ContextTurns {
		turns = turns[len(turns)-opts.ContextTurns:]
	}
	for i, entry := range turns {
		if remaining <= 0 {
			break
		}
		content := entry.item.Get("content")
		if entry.kind == gptAutoKindToolResult {
			content = gptAutoToolOutput(entry.item)
		}
		text, _ := gptAutoContentText(content, capture)
		if strings.TrimSpace(text) == "" {
			continue
		}
		perTurn := remaining / (len(turns) - i)
		role := entry.role
		snapshot.RecentContext = append(snapshot.RecentContext, GPTAutoContextTurn{Role: role, Text: take("recent_context", text, perTurn)})
		if entry.kind == gptAutoKindToolResult {
			limitations = gptAutoAddLimitation(limitations, GPTAutoLimitToolOutputs)
		}
	}
	info.ContextTurnsIncluded = len(snapshot.RecentContext)
	visible += len(snapshot.RecentContext)
	if scan.conversationItems > visible {
		limitations = append(limitations, GPTAutoLimitContextOmitted)
	}
	if latest != nil {
		snapshot.AgentStepsSinceUser = scan.current.steps
	}
	info.UsedChars = opts.MaxChars - remaining
	info.SectionChars = sections
	snapshot.Limitations = limitations
	info.Limitations = limitations
}

// gptAutoActivity summarises the agent loop relevant to the latest user turn
// within a character budget.
func gptAutoActivity(scan *gptAutoScan, opts GPTAutoContextOptions, remaining, limit int) *GPTAutoAgentActivity {
	segment, scope := scan.current, "since_latest_user_request"
	var items []gptAutoScanItem
	latest := scan.latestUser
	switch {
	case latest == nil:
		scope = "visible_items"
		items = scan.tail.ordered()
	case latest.followUp && segment.steps == 0 && scan.previous.steps > 0:
		segment, scope = scan.previous, "before_follow_up"
		for _, entry := range scan.beforeLatest {
			if entry.index > scan.previous.start && entry.index < latest.index {
				items = append(items, entry)
			}
		}
	default:
		for _, entry := range scan.tail.ordered() {
			if entry.index > latest.index {
				items = append(items, entry)
			}
		}
	}
	if segment.steps == 0 {
		return nil
	}
	events := gptAutoRecentEvents(items, scan.toolNames, opts.AgentSteps)
	limit = minInt(limit, remaining)
	for len(events) > 0 && gptAutoEventsChars(events) > limit {
		events = events[1:]
	}
	return &GPTAutoAgentActivity{
		Scope:            scope,
		Steps:            segment.steps,
		ToolCalls:        segment.toolCalls,
		ToolResults:      segment.toolResults,
		FailedResults:    segment.failed,
		TrailingFailures: segment.trailingFailures,
		ToolsUsed:        segment.tools,
		Recent:           events,
		OmittedSteps:     maxInt(segment.steps-len(events), 0),
	}
}

// HasClassifiableContent reports whether the snapshot carries any text or
// activity Jev could classify.
func (s GPTAutoRequestSnapshot) HasClassifiableContent() bool {
	return strings.TrimSpace(s.LatestUser) != "" || len(s.RecentContext) > 0 ||
		(s.AgentActivity != nil && len(s.AgentActivity.Recent) > 0) || s.TaskContext != nil
}

func gptAutoContextInfoPtr(info GPTAutoContextInfo) *GPTAutoContextInfo {
	return &info
}
