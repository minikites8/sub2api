package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIImageStorageArchiverStub struct {
	sources []string
	ids     []string
	errAt   int
}

func (s *openAIImageStorageArchiverStub) ArchiveImage(_ context.Context, sourceURL, objectID string, _ time.Time) (string, error) {
	call := len(s.sources)
	s.sources = append(s.sources, sourceURL)
	s.ids = append(s.ids, objectID)
	if s.errAt >= 0 && call == s.errAt {
		return "", errors.New("upload failed")
	}
	return fmt.Sprintf("https://media.example.com/images/%d.png", call), nil
}

func TestArchiveOpenAIImageResponseURLsArchivesEveryImage(t *testing.T) {
	archiver := &openAIImageStorageArchiverStub{errAt: -1}
	svc := &OpenAIGatewayService{}
	svc.SetGeneratedMediaImageArchiver(archiver)
	body := []byte("{\"created\":1786262400,\"data\":[{\"url\":\"https://upstream.example.com/one.png\",\"revised_prompt\":\"one\"},{\"url\":\"data:image/png;base64,dHdv\"}],\"usage\":{\"images\":2}}")

	rewritten, err := svc.archiveOpenAIImageResponseURLs(context.Background(), body, "req-image")
	require.NoError(t, err)
	require.Equal(t, "https://media.example.com/images/0.png", gjson.GetBytes(rewritten, "data.0.url").String())
	require.Equal(t, "https://media.example.com/images/1.png", gjson.GetBytes(rewritten, "data.1.url").String())
	require.Equal(t, "one", gjson.GetBytes(rewritten, "data.0.revised_prompt").String())
	require.Equal(t, int64(2), gjson.GetBytes(rewritten, "usage.images").Int())
	require.Equal(t, []string{"https://upstream.example.com/one.png", "data:image/png;base64,dHdv"}, archiver.sources)
	require.Equal(t, []string{"image-req-image-0", "image-req-image-1"}, archiver.ids)
}

func TestArchiveOpenAIImageResponseURLsReturnsStorageError(t *testing.T) {
	archiver := &openAIImageStorageArchiverStub{errAt: 1}
	svc := &OpenAIGatewayService{}
	svc.SetGeneratedMediaImageArchiver(archiver)
	body := []byte("{\"data\":[{\"url\":\"https://upstream.example.com/one.png\"},{\"url\":\"https://upstream.example.com/two.png\"}]}")

	rewritten, err := svc.archiveOpenAIImageResponseURLs(context.Background(), body, "req-image")
	require.ErrorContains(t, err, "archive generated image 1")
	require.Nil(t, rewritten)
}

func TestArchiveOpenAIImageSSELineArchivesTopLevelURL(t *testing.T) {
	archiver := &openAIImageStorageArchiverStub{errAt: -1}
	svc := &OpenAIGatewayService{}
	svc.SetGeneratedMediaImageArchiver(archiver)
	line := []byte("data: {\"type\":\"image_generation.completed\",\"created_at\":1786262400,\"url\":\"https://upstream.example.com/result.png\"}\n")

	rewritten, sequence, err := svc.archiveOpenAIImageSSELine(context.Background(), line, "req-stream", 0)
	require.NoError(t, err)
	require.Equal(t, 1, sequence)
	require.Equal(t, "https://media.example.com/images/0.png", gjson.Get(string(rewritten[6:]), "url").String())
	require.Equal(t, []string{"image-req-stream-0"}, archiver.ids)
}
