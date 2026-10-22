package service

// Classification context diagnostics (context-diagnostics-v1).
//
// Diagnostics describe what the state-v3 extractor saw and selected for one
// classification: counts, statuses, character budgets and fixed codes. They
// are produced by gptAutoAssembleContext itself, from the same selection that
// builds the Jev state, so they cannot drift from what Jev was actually sent.
// They never contain request text, tool names, arguments or outputs, and every
// list is drawn from a fixed vocabulary, so the stored object stays bounded.

// GPTAutoContextDiagnosticsVersion identifies the diagnostics schema stored in
// features.context.diagnostics. Rows written before diagnostics existed have
// no diagnostics object at all.
const GPTAutoContextDiagnosticsVersion = "context-diagnostics-v1"

// Scan statuses.
const (
	GPTAutoScanScanned         = "scanned"
	GPTAutoScanSkippedTooLarge = "skipped_too_large"
)

// Selection sections, in the order the builder fills them.
const (
	GPTAutoSectionLatestUser      = "latest_user_request"
	GPTAutoSectionContinues       = "continues_request"
	GPTAutoSectionAgentActivity   = "agent_activity"
	GPTAutoSectionOriginalRequest = "original_request"
	GPTAutoSectionHistorySummary  = "history_summary"
	GPTAutoSectionRecentContext   = "recent_context"
	GPTAutoSectionExternalHistory = "external_history"
)

// Selection step statuses.
const (
	// GPTAutoSelectIncluded: selected and forwarded in full.
	GPTAutoSelectIncluded = "included"
	// GPTAutoSelectTruncated: forwarded, but shortened or partly dropped for
	// the character budget.
	GPTAutoSelectTruncated = "truncated"
	// GPTAutoSelectBudgetExhausted: present in the request, but no budget was
	// left when the builder reached it.
	GPTAutoSelectBudgetExhausted = "budget_exhausted"
	// GPTAutoSelectUnavailable: not contained in the request (held by the
	// provider or removed by compaction).
	GPTAutoSelectUnavailable = "unavailable"
	// GPTAutoSelectNotApplicable: nothing of this kind exists for the request.
	GPTAutoSelectNotApplicable = "not_applicable"
)

// Follow-up resolutions.
const (
	GPTAutoFollowUpNone         = "not_follow_up"
	GPTAutoFollowUpLinked       = "linked_request"
	GPTAutoFollowUpActivityOnly = "agent_activity_only"
	GPTAutoFollowUpUnresolved   = "unresolved"
	// GPTAutoFollowUpRecentTurns: the recent_turns strategy does not link
	// follow-ups; only the preceding turns give them context.
	GPTAutoFollowUpRecentTurns = "recent_turns_only"
)

// Agent activity scopes besides those of GPTAutoAgentActivity.Scope.
const GPTAutoAgentScopeNotSummarized = "not_summarized_by_strategy"

// Diagnostic notes: observations that are not state limitations (Jev is not
// told about them) but matter when reading a decision.
const (
	GPTAutoNoteUnsupportedItems  = "unsupported_items"
	GPTAutoNoteEmptyUserMessages = "empty_user_messages"
	GPTAutoNoteBudgetExhausted   = "context_budget_exhausted"
	GPTAutoNoteAgentEventsTrim   = "agent_events_trimmed"
	GPTAutoNoteFollowUpUnlinked  = "follow_up_unresolved"
	GPTAutoNoteNothingToClassify = "no_classifiable_content"
)

// Completeness levels. They are a deterministic heuristic over the facts
// above, not an estimate of whether Jev understood the request.
const (
	GPTAutoCompletenessSufficient  = "sufficient"
	GPTAutoCompletenessPartial     = "partial"
	GPTAutoCompletenessInsufficent = "important_unavailable"
	GPTAutoCompletenessUnknown     = "unable_to_determine"
)

// GPTAutoContextDiagnostics is the stored diagnostics object.
type GPTAutoContextDiagnostics struct {
	Version    string `json:"diagnostics_version"`
	ScanStatus string `json:"scan_status"`
	// Items is nil when the request was not scanned: its counts are unknown,
	// not zero.
	Items *GPTAutoContextItemCounts `json:"items,omitempty"`
	// Selection lists the builder's steps in the order it ran them.
	Selection []GPTAutoContextSelectionStep `json:"selection,omitempty"`
	// ContextTruncated is true when any selected text was shortened or any
	// available item was dropped for the character budget.
	ContextTruncated bool                       `json:"context_truncated"`
	FollowUp         GPTAutoFollowUpDiagnostics `json:"follow_up"`
	// Agent is nil when no agent steps relate to the latest user message.
	Agent               *GPTAutoAgentDiagnostics `json:"agent,omitempty"`
	Notes               []string                 `json:"notes,omitempty"`
	Completeness        string                   `json:"completeness"`
	CompletenessReasons []string                 `json:"completeness_reasons,omitempty"`
	// StateChars is the length of the serialized Jev state. It is set where
	// the state is serialized (the shadow worker, the debugger); nil otherwise.
	StateChars *int `json:"state_chars,omitempty"`
}

// GPTAutoContextItemCounts accounts for every item of the request's
// conversation array. Each item lands in exactly one of Eligible, Privacy,
// Unsupported, ExternalReferences, Compacted or Empty; eligible items are
// further split into Represented, BudgetDropped and NotSelected.
type GPTAutoContextItemCounts struct {
	// Eligible items carry text the extractor reads: user and assistant
	// messages, tool calls, tool results and client history summaries.
	Eligible int `json:"eligible"`
	// Represented eligible items reached the state as text or as an agent
	// event summary.
	Represented int `json:"represented"`
	// SelectedMessages counts message excerpts in the state (latest request,
	// continued request, original request, history summary, recent turns).
	SelectedMessages int `json:"selected_messages"`
	// BudgetDropped eligible items were selected but dropped because the
	// character budget (or a section's share of it) ran out.
	BudgetDropped int `json:"budget_dropped"`
	// BudgetTruncated counts forwarded excerpts that were shortened.
	BudgetTruncated int `json:"budget_truncated"`
	// NotSelected eligible items fell outside the strategy's turn and step
	// windows (older turns, older agent steps).
	NotSelected int `json:"not_selected"`
	// Privacy: system/developer turns, client harness messages and reasoning
	// items, whose text is withheld by design.
	Privacy int `json:"privacy"`
	// Unsupported: item types or roles the extractor does not read.
	Unsupported int `json:"unsupported"`
	// ExternalReferences: item_reference items (content held by the provider).
	ExternalReferences int `json:"external_references"`
	// Compacted: opaque compaction items.
	Compacted int `json:"compacted"`
	// Empty: user messages with neither text nor images.
	Empty int `json:"empty"`
	// WithImages: eligible user messages carrying images (never forwarded).
	WithImages int `json:"with_images"`
	// ConversationLengthKnown is false when earlier history is held by the
	// provider (previous_response_id, conversation, item_reference): the
	// request then shows only part of the conversation.
	ConversationLengthKnown bool `json:"conversation_length_known"`
}

// GPTAutoContextSelectionStep is one builder step.
type GPTAutoContextSelectionStep struct {
	Section string `json:"section"`
	Status  string `json:"status"`
	// Chars is the budget the step consumed (agent activity is charged for
	// its event summaries plus a fixed per-event overhead).
	Chars int `json:"chars"`
	// Items: messages (or agent events) forwarded / available / dropped for
	// the budget. Zero values are omitted.
	Items     int `json:"items,omitempty"`
	Available int `json:"available,omitempty"`
	Dropped   int `json:"dropped,omitempty"`
}

// GPTAutoFollowUpDiagnostics describes short follow-up handling.
type GPTAutoFollowUpDiagnostics struct {
	Resolution string `json:"resolution"`
	// ShortMessage: the latest user message is short enough to be a
	// follow-up. Together with resolution not_follow_up it marks a short
	// message the detector did not treat as a continuation.
	ShortMessage           bool `json:"short_message"`
	OriginalTaskAvailable  bool `json:"original_task_available"`
	AgentActivityAvailable bool `json:"agent_activity_available"`
}

// GPTAutoAgentDiagnostics counts the agent loop around the latest user
// message. Tool names, arguments and outputs are never recorded.
type GPTAutoAgentDiagnostics struct {
	Scope                string `json:"scope"`
	StepLimit            int    `json:"step_limit"`
	StepsDetected        int    `json:"steps_detected"`
	ToolCallsDetected    int    `json:"tool_calls_detected"`
	ToolResultsDetected  int    `json:"tool_results_detected"`
	ToolFailuresDetected int    `json:"tool_failures_detected"`
	TrailingFailures     int    `json:"trailing_failures"`
	// FailureSummariesIncluded counts failed tool results among the
	// forwarded events.
	FailureSummariesIncluded int `json:"failure_summaries_included"`
	EventsDroppedForBudget   int `json:"events_dropped_for_budget"`
	// EventKinds counts forwarded events by kind (tool_call, tool_result_ok,
	// tool_result_error, tool_result_unknown, assistant_note).
	EventKinds           map[string]int `json:"event_kinds,omitempty"`
	HasToolResultContext bool           `json:"has_tool_result_context"`
	HasProgressNote      bool           `json:"has_progress_note"`
}

// gptAutoEventKind is the diagnostic category of a forwarded agent event.
func gptAutoEventKind(event GPTAutoAgentEvent) string {
	if event.Kind != "tool_result" {
		return event.Kind
	}
	switch event.Status {
	case "error", "ok":
		return "tool_result_" + event.Status
	}
	return "tool_result_unknown"
}

// gptAutoIndexSet is a small set of item indices (selections are bounded by
// the turn and step limits, so a slice is cheaper than a map).
type gptAutoIndexSet []int

func (s *gptAutoIndexSet) add(index int) bool {
	if index < 0 {
		return false
	}
	for _, existing := range *s {
		if existing == index {
			return false
		}
	}
	*s = append(*s, index)
	return true
}

func (s gptAutoIndexSet) has(index int) bool {
	for _, existing := range s {
		if existing == index {
			return true
		}
	}
	return false
}

// gptAutoCompletenessPartialCodes are the limitation and note codes that make
// observed context partial, in display order.
var gptAutoCompletenessPartialCodes = []string{
	GPTAutoLimitLatestUserTrimmed, GPTAutoLimitHistoryByReference, GPTAutoLimitItemReferences,
	GPTAutoLimitHistoryCompacted, GPTAutoNoteBudgetExhausted, GPTAutoNoteAgentEventsTrim,
	GPTAutoLimitContextOmitted, GPTAutoLimitImagesNotForwarded, GPTAutoNoteUnsupportedItems,
}

// gptAutoCompleteness applies the documented heuristic:
//   - unable_to_determine: the request was not scanned, or nothing in it
//     could be classified;
//   - important_unavailable: no user message was found, or a follow-up could
//     not be linked to any earlier request or agent activity;
//   - partial: something the request refers to or contains did not reach the
//     state (trimmed latest message, provider-held or compacted history,
//     budget drops, older turns outside the window, images, unsupported items);
//   - sufficient: otherwise. Intentional exclusions (system prompts, raw tool
//     outputs) do not count against completeness.
func gptAutoCompleteness(d *GPTAutoContextDiagnostics, limitations []string, classifiable bool) {
	codes := append(append([]string(nil), limitations...), d.Notes...)
	switch {
	case d.ScanStatus != GPTAutoScanScanned:
		d.Completeness, d.CompletenessReasons = GPTAutoCompletenessUnknown, []string{GPTAutoLimitRequestTooLarge}
		return
	case !classifiable:
		d.Completeness, d.CompletenessReasons = GPTAutoCompletenessUnknown, []string{GPTAutoNoteNothingToClassify}
		return
	}
	var important []string
	for _, code := range []string{GPTAutoLimitNoUserMessage, GPTAutoNoteFollowUpUnlinked} {
		if containsString(codes, code) {
			important = append(important, code)
		}
	}
	if len(important) > 0 {
		d.Completeness, d.CompletenessReasons = GPTAutoCompletenessInsufficent, important
		return
	}
	var partial []string
	for _, code := range gptAutoCompletenessPartialCodes {
		if containsString(codes, code) {
			partial = append(partial, code)
		}
	}
	if len(partial) > 0 {
		d.Completeness, d.CompletenessReasons = GPTAutoCompletenessPartial, partial
		return
	}
	d.Completeness = GPTAutoCompletenessSufficient
}

// gptAutoSkippedDiagnostics describes a request that was not scanned.
func gptAutoSkippedDiagnostics() *GPTAutoContextDiagnostics {
	d := &GPTAutoContextDiagnostics{
		Version:    GPTAutoContextDiagnosticsVersion,
		ScanStatus: GPTAutoScanSkippedTooLarge,
		FollowUp:   GPTAutoFollowUpDiagnostics{Resolution: GPTAutoFollowUpNone},
	}
	gptAutoCompleteness(d, nil, false)
	return d
}

// withStateChars returns a copy of the context info whose diagnostics carry
// the serialized state length.
func (info GPTAutoContextInfo) withStateChars(chars int) GPTAutoContextInfo {
	if info.Diagnostics != nil {
		diagnostics := *info.Diagnostics
		diagnostics.StateChars = &chars
		info.Diagnostics = &diagnostics
	}
	return info
}
