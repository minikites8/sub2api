package service

import "fmt"

// GPTQoSSelectionError carries a safe client reason separately from the
// server-only recheck detail. It never contains credentials or request bodies.
type GPTQoSSelectionError struct {
	Reason string
	detail string
	cause  error
}

func newGPTQoSSelectionError(reason, detail string, cause error) error {
	return &GPTQoSSelectionError{Reason: reason, detail: detail, cause: cause}
}

func (e *GPTQoSSelectionError) Error() string {
	if e.detail != "" {
		return fmt.Sprintf("%s: QoS %s (%s)", ErrNoAvailableAccounts, e.Reason, e.detail)
	}
	return fmt.Sprintf("%s: QoS %s", ErrNoAvailableAccounts, e.Reason)
}

func (e *GPTQoSSelectionError) Unwrap() []error {
	if e.cause != nil {
		return []error{ErrNoAvailableAccounts, e.cause}
	}
	return []error{ErrNoAvailableAccounts}
}

// ClientError is deliberately allowlisted. Detailed account/configuration
// changes belong to operator logs, not public API responses.
func (e *GPTQoSSelectionError) ClientError() (code, message string) {
	switch e.Reason {
	case "account_changed":
		return "gpt_qos_account_changed", "Eligible account configuration changed during scheduling. Please retry."
	case "no_eligible_capacity":
		return "gpt_qos_no_eligible_capacity", "No eligible accounts with supported, bounded capacity are available under this group's quality policy."
	case "queue_full":
		return "gpt_qos_queue_full", "The scheduling queue is full. Please retry later."
	case "queue_timeout":
		return "gpt_qos_queue_timeout", "No eligible account capacity became available within the scheduling wait budget. Please retry later."
	case "queue_closed":
		return "gpt_qos_queue_closed", "Scheduling admission was closed before capacity could be assigned. Please retry."
	case "candidate_limit":
		return "gpt_qos_candidate_limit", "The eligible account pool exceeds the scheduling queue limit. Please contact the administrator."
	case "snapshot_unavailable":
		return "gpt_qos_snapshot_unavailable", "Account scheduling state is temporarily unavailable. Please retry later."
	default:
		return "gpt_qos_unavailable", "Fair scheduling admission is temporarily unavailable. Please retry later."
	}
}
