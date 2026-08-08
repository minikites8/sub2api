package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type openAIImageURLCandidate struct {
	path      string
	sourceURL string
}

func (s *OpenAIGatewayService) SetGeneratedMediaImageArchiver(archiver GeneratedMediaImageArchiver) {
	if s == nil {
		return
	}
	s.imageMediaStore = archiver
}

func (s *OpenAIGatewayService) archiveOpenAIImageResponseURLs(ctx context.Context, body []byte, requestID string) ([]byte, error) {
	archived, _, err := s.archiveOpenAIImagePayloadURLs(ctx, body, requestID, 0)
	return archived, err
}

func (s *OpenAIGatewayService) archiveOpenAIImageSSELine(ctx context.Context, line []byte, requestID string, sequence int) ([]byte, int, error) {
	if s == nil || s.imageMediaStore == nil || len(line) == 0 {
		return line, sequence, nil
	}
	rawLine := strings.TrimRight(string(line), "\r\n")
	data, ok := extractOpenAISSEDataLine(rawLine)
	if !ok || !gjson.Valid(data) {
		return line, sequence, nil
	}
	rewritten, nextSequence, err := s.archiveOpenAIImagePayloadURLs(ctx, []byte(data), requestID, sequence)
	if err != nil {
		return nil, sequence, err
	}
	if nextSequence == sequence {
		return line, sequence, nil
	}
	prefixLength := len(rawLine) - len(data)
	lineEnding := string(line[len(rawLine):])
	return []byte(rawLine[:prefixLength] + string(rewritten) + lineEnding), nextSequence, nil
}

func (s *OpenAIGatewayService) archiveOpenAIImagePayloadURLs(ctx context.Context, payload []byte, requestID string, sequence int) ([]byte, int, error) {
	if s == nil || s.imageMediaStore == nil || len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload, sequence, nil
	}

	candidates := make([]openAIImageURLCandidate, 0, 2)
	if sourceURL := strings.TrimSpace(gjson.GetBytes(payload, "url").String()); sourceURL != "" {
		candidates = append(candidates, openAIImageURLCandidate{path: "url", sourceURL: sourceURL})
	}
	if items := gjson.GetBytes(payload, "data"); items.Exists() && items.IsArray() {
		for index, item := range items.Array() {
			if sourceURL := strings.TrimSpace(item.Get("url").String()); sourceURL != "" {
				candidates = append(candidates, openAIImageURLCandidate{
					path:      fmt.Sprintf("data.%d.url", index),
					sourceURL: sourceURL,
				})
			}
		}
	}
	if len(candidates) == 0 {
		return payload, sequence, nil
	}

	createdAt := time.Now()
	for _, path := range []string{"created", "created_at"} {
		if timestamp := gjson.GetBytes(payload, path).Int(); timestamp > 0 {
			createdAt = time.Unix(timestamp, 0)
			break
		}
	}
	baseID := strings.TrimSpace(requestID)
	if baseID == "" {
		baseID = uuid.NewString()
	}

	archived := payload
	for _, candidate := range candidates {
		archivedURL, err := s.imageMediaStore.ArchiveImage(
			ctx,
			candidate.sourceURL,
			fmt.Sprintf("image-%s-%d", baseID, sequence),
			createdAt,
		)
		if err != nil {
			return nil, sequence, fmt.Errorf("archive generated image %d: %w", sequence, err)
		}
		archived, err = sjson.SetBytes(archived, candidate.path, archivedURL)
		if err != nil {
			return nil, sequence, fmt.Errorf("rewrite generated image URL %d: %w", sequence, err)
		}
		sequence++
	}
	return archived, sequence, nil
}

func (s *OpenAIGatewayService) archiveOpenAIResponsesImageResult(
	ctx context.Context,
	image *openAIResponsesImageResult,
	requestID string,
	index int,
	createdAt int64,
) error {
	if s == nil || s.imageMediaStore == nil || image == nil {
		return nil
	}
	if strings.TrimSpace(image.ArchivedURL) != "" {
		return nil
	}
	result := strings.TrimSpace(image.Result)
	if result == "" {
		return nil
	}
	mediaType := openAIImageOutputMIMEType(image.OutputFormat)
	sourceURL := "data:" + mediaType + ";base64," + result
	archiveTime := time.Now()
	if createdAt > 0 {
		archiveTime = time.Unix(createdAt, 0)
	}
	baseID := strings.TrimSpace(requestID)
	if baseID == "" {
		baseID = uuid.NewString()
	}
	archivedURL, err := s.imageMediaStore.ArchiveImage(
		ctx,
		sourceURL,
		fmt.Sprintf("image-%s-%d", baseID, index),
		archiveTime,
	)
	if err != nil {
		return fmt.Errorf("archive generated image %d: %w", index, err)
	}
	image.ArchivedURL = archivedURL
	return nil
}
