package service

import (
	"context"
	"log/slog"
	"strings"
)

// gptAutoClassifierHistoryLimit bounds the version list returned to the UI.
const gptAutoClassifierHistoryLimit = 50

// GPTAutoClassifierLimits are the validation bounds the UI mirrors.
type GPTAutoClassifierLimits struct {
	MinRunes int `json:"min_runes"`
	MaxRunes int `json:"max_runes"`
}

// GPTAutoClassifierView is the admin view of the Jev classification
// instructions: the saved text, the shipped defaults, the effective questions
// Jev receives, and the stored version history.
type GPTAutoClassifierView struct {
	Instructions GPTAutoClassifierInstructions   `json:"instructions"`
	Defaults     GPTAutoClassifierInstructions   `json:"defaults"`
	IsDefault    bool                            `json:"is_default"`
	Hash         string                          `json:"hash"`
	Effective    GPTAutoClassifierSnapshot       `json:"effective"`
	History      []GPTAutoClassifierConfigRecord `json:"history"`
	Limits       GPTAutoClassifierLimits         `json:"limits"`
	// LegacyInstructions are the v1 hard-coded instructions used by decisions
	// that have no classifier hash.
	LegacyInstructions map[string]string `json:"legacy_instructions"`
}

// GPTAutoClassifierPreview is the effective question set for a draft. No Jev
// call is made.
type GPTAutoClassifierPreview struct {
	Hash        string                    `json:"hash"`
	Effective   GPTAutoClassifierSnapshot `json:"effective"`
	SameAsSaved bool                      `json:"same_as_saved"`
}

// ClassifierView returns the saved instructions and their history.
func (s *GPTAutoShadowService) ClassifierView(ctx context.Context) (*GPTAutoClassifierView, error) {
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := NewGPTAutoClassifierSnapshot(*cfg)
	hash, _, err := snapshot.Hash()
	if err != nil {
		return nil, err
	}
	view := &GPTAutoClassifierView{
		Instructions:       cfg.ClassifierInstructions,
		Defaults:           DefaultGPTAutoClassifierInstructions(),
		IsDefault:          sameGPTAutoInstructionText(cfg.ClassifierInstructions, DefaultGPTAutoClassifierInstructions()),
		Hash:               hash,
		Effective:          snapshot,
		History:            []GPTAutoClassifierConfigRecord{},
		Limits:             GPTAutoClassifierLimits{MinRunes: gptAutoMinInstructionRunes, MaxRunes: gptAutoMaxInstructionRunes},
		LegacyInstructions: gptAutoLegacyQuestionInstructions,
	}
	if s.repo != nil {
		history, err := s.repo.ListClassifierConfigs(ctx, gptAutoClassifierHistoryLimit)
		if err != nil {
			slog.Warn("gpt_auto.classifier_history_failed", "error", err)
		} else if history != nil {
			view.History = history
		}
	}
	return view, nil
}

// PreviewClassifier returns the effective questions for draft instructions.
func (s *GPTAutoShadowService) PreviewClassifier(ctx context.Context, draft GPTAutoClassifierInstructions) (*GPTAutoClassifierPreview, error) {
	if err := ValidateGPTAutoClassifierInstructions(draft); err != nil {
		return nil, err
	}
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	same := sameGPTAutoInstructionText(cfg.ClassifierInstructions, draft)
	cfg.ClassifierInstructions = draft
	normalizeGPTAutoClassifierInstructions(&cfg.ClassifierInstructions)
	snapshot := NewGPTAutoClassifierSnapshot(*cfg)
	hash, _, err := snapshot.Hash()
	if err != nil {
		return nil, err
	}
	return &GPTAutoClassifierPreview{Hash: hash, Effective: snapshot, SameAsSaved: same}, nil
}

// SaveClassifierInstructions validates and stores new instructions, bumps the
// revision and records the version (with its author) in the history table.
// New classifications pick the change up from the config cache: immediately
// on this node, within gptAutoConfigCacheTTL on the others. Saving unchanged
// text is a no-op.
func (s *GPTAutoShadowService) SaveClassifierInstructions(ctx context.Context, next GPTAutoClassifierInstructions, author *int64) (*GPTAutoClassifierView, error) {
	if err := ValidateGPTAutoClassifierInstructions(next); err != nil {
		return nil, err
	}
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !sameGPTAutoInstructionText(cfg.ClassifierInstructions, next) {
		now := s.now().UTC()
		cfg.ClassifierInstructions = GPTAutoClassifierInstructions{
			ModelSelection:         strings.TrimSpace(next.ModelSelection),
			EffortSelection:        strings.TrimSpace(next.EffortSelection),
			ComplexityScoring:      strings.TrimSpace(next.ComplexityScoring),
			CategoryClassification: strings.TrimSpace(next.CategoryClassification),
			Revision:               cfg.ClassifierInstructions.Revision + 1,
			UpdatedAt:              &now,
			UpdatedBy:              cloneInt64Ptr(author),
		}
		if err := s.settings.SetGPTAutoShadowConfig(ctx, cfg); err != nil {
			return nil, err
		}
		s.RecordClassifierConfig(ctx, author)
	}
	return s.ClassifierView(ctx)
}

// RecordClassifierConfig stores the currently saved classifier configuration
// in the history table, attributed to author. Called after any admin save
// that can change the questions (instructions or model descriptions).
func (s *GPTAutoShadowService) RecordClassifierConfig(ctx context.Context, author *int64) {
	if s == nil || s.repo == nil {
		return
	}
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		slog.Warn("gpt_auto.classifier_config_record_failed", "error", err)
		return
	}
	if err := s.ensureClassifierConfig(ctx, *cfg, author); err != nil {
		slog.Warn("gpt_auto.classifier_config_record_failed", "error", err)
	}
}
