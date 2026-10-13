package service

import "context"

// Manual, automatic and donor validation share the same account/model queue.
// A parallel task owns these keys once; its coordinator publishes winning tickets.
func (s *OpenAIGatewayService) lockCodexHarvest(ctx context.Context, id int64, models []string) (func(), error) {
	keys := make([]string, 0, len(models))
	for _, model := range models {
		keys = append(keys, openAICodexTicketKey(id, normalizeOpenAICodexTicketModel(model)))
	}
	return s.codexHarvestRuns.AcquireMany(ctx, keys)
}
