package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	_ "golang.org/x/image/webp"
	"golang.org/x/sync/singleflight"
)

const (
	excelBPSImageTTL                 = 24 * time.Hour
	excelBPSImageCacheLimit          = 512
	excelBPSMaxImageBytes      int64 = 20 << 20
	excelBPSMaxBatchImageBytes       = 64 << 20
)

// BasispointsImageFileCache optionally persists account-scoped attachment IDs.
// Expiration is absolute so replaying an image preserves the upload's lifetime.
type BasispointsImageFileCache interface {
	GetBasispointsImageFile(context.Context, string, string) (string, time.Time, error)
	SetBasispointsImageFile(context.Context, string, string, string, time.Time) error
}

type excelBPSImageError struct {
	status  int
	message string
}

func (e *excelBPSImageError) Error() string { return e.message }

type excelBPSImageEntry struct {
	fileID    string
	expiresAt time.Time
	usedAt    time.Time
}

type excelBPSImageCache struct {
	mu      sync.Mutex
	entries map[string]excelBPSImageEntry
	uploads singleflight.Group
}

func (c *excelBPSImageCache) get(key string) (excelBPSImageEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !time.Now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return excelBPSImageEntry{}, false
	}
	entry.usedAt = time.Now()
	c.entries[key] = entry
	return entry, true
}

func (c *excelBPSImageCache) put(key string, entry excelBPSImageEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]excelBPSImageEntry)
	}
	now := time.Now()
	for k, value := range c.entries {
		if !now.Before(value.expiresAt) {
			delete(c.entries, k)
		}
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= excelBPSImageCacheLimit {
		oldestKey := ""
		var oldest time.Time
		for k, value := range c.entries {
			if oldestKey == "" || value.usedAt.Before(oldest) {
				oldestKey, oldest = k, value.usedAt
			}
		}
		delete(c.entries, oldestKey)
	}
	entry.usedAt = now
	c.entries[key] = entry
}

func excelBPSImageScope(account *Account, accountID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", account.ID, accountID)))
	return hex.EncodeToString(sum[:])
}

func validExcelBPSFileID(id string) bool {
	return basispoints.ValidateImage(map[string]any{"file_id": id}) == nil
}

func (s *OpenAIGatewayService) excelBPSImageFileID(ctx context.Context, account *Account, token, accountID, contentType string, data []byte) (string, error) {
	scope := excelBPSImageScope(account, accountID)
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	key := scope + ":" + digest
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if entry, ok := s.excelBPSImages.get(key); ok {
		return entry.fileID, nil
	}
	done := s.excelBPSImages.uploads.DoChan(key, func() (any, error) {
		if entry, ok := s.excelBPSImages.get(key); ok {
			return entry.fileID, nil
		}
		persistent, _ := s.cache.(BasispointsImageFileCache)
		if persistent != nil {
			cacheCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			id, expiresAt, err := persistent.GetBasispointsImageFile(cacheCtx, scope, digest)
			cancel()
			if err == nil && time.Now().Before(expiresAt) && validExcelBPSFileID(id) {
				s.excelBPSImages.put(key, excelBPSImageEntry{fileID: id, expiresAt: expiresAt})
				return id, nil
			}
		}
		id, err := s.uploadExcelBPSImage(ctx, account, token, accountID, contentType, data)
		if err != nil {
			return "", err
		}
		entry := excelBPSImageEntry{fileID: id, expiresAt: time.Now().Add(excelBPSImageTTL)}
		s.excelBPSImages.put(key, entry)
		if persistent != nil {
			cacheCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = persistent.SetBasispointsImageFile(cacheCtx, scope, digest, id, entry.expiresAt)
			cancel()
		}
		return id, nil
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-done:
		if result.Err != nil {
			return "", result.Err
		}
		return result.Val.(string), nil
	}
}

func (s *OpenAIGatewayService) uploadExcelBPSImage(ctx context.Context, account *Account, token, accountID, contentType string, data []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", "vision"); err != nil {
		return "", err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": "image" + extensionForContentType(contentType)}))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	req, err := newExcelBPSRequest(ctx, body.Bytes(), token, accountID)
	if err != nil {
		return "", err
	}
	req.URL.Path = "/basispoints/api/attachments"
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Copilot-Vision-Request", "true")
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return "", &excelBPSImageError{http.StatusBadGateway, "Basispoints image upload transport failed"}
	}
	if resp == nil || resp.Body == nil {
		return "", &excelBPSImageError{http.StatusBadGateway, "Basispoints image upload returned an empty response"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &excelBPSImageError{http.StatusBadGateway, fmt.Sprintf("Basispoints image upload returned HTTP %d", resp.StatusCode)}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return "", &excelBPSImageError{http.StatusBadGateway, "Basispoints image upload returned an invalid response"}
	}
	var response map[string]json.RawMessage
	if json.Unmarshal(raw, &response) == nil {
		for _, field := range []string{"openai_file_id", "file_id", "id"} {
			var id string
			if json.Unmarshal(response[field], &id) == nil && validExcelBPSFileID(id) {
				return id, nil
			}
		}
	}
	return "", &excelBPSImageError{http.StatusBadGateway, "Basispoints image upload requires a valid file ID in the response"}
}

// materializeExcelBPSImages rewrites only image blocks in the Responses input.
// This includes historical messages and structured tool outputs. Text, HTTPS
// URLs, existing attachment IDs and the caller's original bytes stay intact.
func (s *OpenAIGatewayService) materializeExcelBPSImages(ctx context.Context, body []byte, account *Account, token, accountID string) ([]byte, bool, error) {
	var source map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if !json.Valid(body) || decoder.Decode(&source) != nil {
		return nil, false, &excelBPSImageError{http.StatusBadRequest, "Invalid image request JSON"}
	}
	var images []map[string]any
	var visit func(any, int) error
	visit = func(value any, depth int) error {
		if depth > 128 {
			return &excelBPSImageError{http.StatusBadRequest, "Image input nesting exceeds 128 levels"}
		}
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		case map[string]any:
			if item["type"] == "input_image" {
				images = append(images, item)
				return nil
			}
			for _, child := range item {
				if err := visit(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(source["input"], 0); err != nil {
		return nil, false, err
	}
	type upload struct {
		data        []byte
		contentType string
		parts       []map[string]any
	}
	var pending []*upload
	seen := make(map[[32]byte]*upload)
	var total int
	imageDecoder := &ImageResultUploader{maxDownloadBytes: excelBPSMaxImageBytes}
	for _, part := range images {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}
		rawURL, _ := part["image_url"].(string)
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(rawURL)), "data:") {
			if err := basispoints.ValidateImage(part); err != nil {
				return nil, true, &excelBPSImageError{http.StatusBadRequest, err.Error()}
			}
			continue
		}
		if _, present := part["file_id"]; present {
			return nil, true, &excelBPSImageError{http.StatusBadRequest, "An image must specify exactly one of image_url or file_id"}
		}
		check := map[string]any{"file_id": "file-validation"}
		if detail, exists := part["detail"]; exists {
			check["detail"] = detail
		}
		if err := basispoints.ValidateImage(check); err != nil {
			return nil, true, &excelBPSImageError{http.StatusBadRequest, err.Error()}
		}
		data, contentType, err := imageDecoder.decodeImageDataURL(strings.TrimSpace(rawURL))
		if err != nil || len(data) == 0 {
			return nil, true, &excelBPSImageError{http.StatusBadRequest, "Inline image must be valid base64 image data of at most 20 MiB"}
		}
		switch contentType {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
		default:
			return nil, true, &excelBPSImageError{http.StatusBadRequest, "Inline images must use PNG, JPEG, GIF or WebP"}
		}
		info, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || info.Width <= 0 || info.Height <= 0 || int64(info.Width)*int64(info.Height) > 100000000 {
			return nil, true, &excelBPSImageError{http.StatusBadRequest, "Inline image must have valid dimensions within 100 megapixels"}
		}
		digest := sha256.Sum256(data)
		if existing := seen[digest]; existing != nil {
			existing.parts = append(existing.parts, part)
			continue
		}
		total += len(data)
		if total > excelBPSMaxBatchImageBytes {
			return nil, true, &excelBPSImageError{http.StatusRequestEntityTooLarge, "Decoded inline images exceed 64 MiB per request"}
		}
		item := &upload{data: data, contentType: contentType, parts: []map[string]any{part}}
		seen[digest] = item
		pending = append(pending, item)
	}
	if len(pending) == 0 {
		return body, len(images) > 0, nil
	}
	for _, item := range pending {
		id, err := s.excelBPSImageFileID(ctx, account, token, accountID, item.contentType, item.data)
		if err != nil {
			return nil, true, err
		}
		for _, part := range item.parts {
			delete(part, "image_url")
			part["file_id"] = id
		}
	}
	rewritten, err := json.Marshal(source)
	return rewritten, true, err
}
