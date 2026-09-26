package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type excelBPSImageUpstream struct {
	HTTPUpstream
	mu           sync.Mutex
	requests     []*http.Request
	bodies       [][]byte
	proxies      []string
	accounts     []int64
	uploads      int
	uploadStatus int
	uploadJSON   string
	uploadError  error
	delay        time.Duration
}

func (u *excelBPSImageUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	u.mu.Lock()
	u.requests = append(u.requests, req)
	u.bodies = append(u.bodies, body)
	u.proxies = append(u.proxies, proxyURL)
	u.accounts = append(u.accounts, accountID)
	upload := req.URL.Path == "/basispoints/api/attachments"
	if upload {
		u.uploads++
	}
	count := u.uploads
	u.mu.Unlock()
	if upload {
		if u.delay > 0 {
			select {
			case <-time.After(u.delay):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		if u.uploadError != nil {
			return nil, u.uploadError
		}
		status := u.uploadStatus
		if status == 0 {
			status = http.StatusOK
		}
		payload := u.uploadJSON
		if payload == "" {
			payload = fmt.Sprintf(`{"openai_file_id":"file-upload-%d"}`, count)
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}
	event := `event: response.completed
data: {"type":"response.completed","response":{"id":"resp_image","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}

`
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(event))}, nil
}

func excelBPSImageFixture(t *testing.T) (string, []byte) {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes()), output.Bytes()
}

func excelBPSImageBody(t *testing.T, parts ...map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": "gpt-5.6-sol", "input": []any{map[string]any{"type": "message", "role": "user", "content": parts}}})
	require.NoError(t, err)
	return body
}

func newExcelBPSImageTestService(upstream *excelBPSImageUpstream) *OpenAIGatewayService {
	svc := openAIClientToolsTestService(&httpUpstreamRecorder{})
	svc.httpUpstream = upstream
	return svc
}

func TestExcelBPSNativeImageForwardAndReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			dataURL, raw := excelBPSImageFixture(t)
			upstream := &excelBPSImageUpstream{}
			svc := newExcelBPSImageTestService(upstream)
			account := excelAccount()
			account.Proxy = &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8888}
			body := excelBPSImageBody(t, map[string]any{"type": "input_image", "image_url": dataURL, "detail": "high"})
			var request map[string]any
			require.NoError(t, json.Unmarshal(body, &request))
			request["stream"] = stream
			body, _ = json.Marshal(request)
			original := bytes.Clone(body)
			for i := 0; i < 2; i++ {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				c.Request.Header.Set("Cookie", "private-cookie")
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 200, recorder.Code)
			}
			require.Equal(t, original, body)
			require.Equal(t, 1, upstream.uploads)
			require.Len(t, upstream.requests, 3)
			for i, req := range upstream.requests {
				require.Equal(t, "bps.openai.com", req.URL.Host)
				require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
				require.Equal(t, "test-account", req.Header.Get("Chatgpt-Account-Id"))
				require.Equal(t, "true", req.Header.Get("Copilot-Vision-Request"))
				require.Empty(t, req.Header.Get("Cookie"))
				require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
				require.Equal(t, account.Proxy.URL(), upstream.proxies[i])
				require.Equal(t, account.ID, upstream.accounts[i])
			}
			_, params, err := mime.ParseMediaType(upstream.requests[0].Header.Get("Content-Type"))
			require.NoError(t, err)
			form, err := multipart.NewReader(bytes.NewReader(upstream.bodies[0]), params["boundary"]).ReadForm(1 << 20)
			require.NoError(t, err)
			defer form.RemoveAll()
			require.Equal(t, []string{"vision"}, form.Value["purpose"])
			require.Len(t, form.File["file"], 1)
			file, err := form.File["file"][0].Open()
			require.NoError(t, err)
			uploaded, err := io.ReadAll(file)
			require.NoError(t, err)
			require.NoError(t, file.Close())
			require.Equal(t, raw, uploaded)
			require.Equal(t, "image/png", form.File["file"][0].Header.Get("Content-Type"))
			for _, forwarded := range upstream.bodies[1:] {
				require.NotContains(t, string(forwarded), "data:image")
				require.Contains(t, string(forwarded), "file-upload-1")
				require.Contains(t, string(forwarded), `"detail":"high"`)
			}
			require.Equal(t, gjson.GetBytes(upstream.bodies[1], "metadata").Raw, gjson.GetBytes(upstream.bodies[2], "metadata").Raw)
		})
	}
}

func TestExcelBPSImagesValidationAndPassthrough(t *testing.T) {
	dataURL, _ := excelBPSImageFixture(t)
	for name, part := range map[string]map[string]any{
		"malformed": {"type": "input_image", "image_url": "data:image/png;base64,PRIVATE_IMAGE_BYTES"},
		"empty":     {"type": "input_image", "image_url": "data:image/png;base64,"},
		"fake png":  {"type": "input_image", "image_url": "data:image/png;base64,aGVsbG8="},
		"svg":       {"type": "input_image", "image_url": "data:image/svg+xml;base64,PHN2Zy8+"},
		"detail":    {"type": "input_image", "image_url": dataURL, "detail": "original"},
		"mixed":     {"type": "input_image", "image_url": dataURL, "file_id": "file-existing"},
		"http":      {"type": "input_image", "image_url": "http://example.test/a.png"},
	} {
		t.Run(name, func(t *testing.T) {
			upstream := &excelBPSImageUpstream{}
			svc := newExcelBPSImageTestService(upstream)
			body := excelBPSImageBody(t, map[string]any{"type": "input_image", "image_url": dataURL}, part)
			_, _, err := svc.materializeExcelBPSImages(context.Background(), body, excelAccount(), "token", "account")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "PRIVATE_IMAGE_BYTES")
			require.Empty(t, upstream.requests)
		})
	}
	for _, part := range []map[string]any{
		{"type": "input_image", "image_url": "https://images.example/a.png?signature=unchanged%2Fvalue"},
		{"type": "input_image", "file_id": "file-existing", "detail": "low"},
	} {
		upstream := &excelBPSImageUpstream{}
		svc := newExcelBPSImageTestService(upstream)
		body := excelBPSImageBody(t, part)
		out, vision, err := svc.materializeExcelBPSImages(context.Background(), body, excelAccount(), "token", "account")
		require.NoError(t, err)
		require.True(t, vision)
		require.Equal(t, body, out)
		require.Empty(t, upstream.requests)
	}
}

func TestExcelBPSImagesHistoryAndToolOutput(t *testing.T) {
	dataURL, _ := excelBPSImageFixture(t)
	imagePart := map[string]any{"type": "input_image", "image_url": dataURL}
	source := map[string]any{"input": []any{map[string]any{"role": "user", "content": []any{imagePart}}, map[string]any{"type": "function_call_output", "output": []any{imagePart}}}, "large_number": json.Number("9007199254740993")}
	body, _ := json.Marshal(source)
	upstream := &excelBPSImageUpstream{}
	svc := newExcelBPSImageTestService(upstream)
	out, vision, err := svc.materializeExcelBPSImages(context.Background(), body, excelAccount(), "token", "account")
	require.NoError(t, err)
	require.True(t, vision)
	require.Equal(t, 1, upstream.uploads)
	require.Equal(t, "file-upload-1", gjson.GetBytes(out, "input.0.content.0.file_id").String())
	require.Equal(t, "file-upload-1", gjson.GetBytes(out, "input.1.output.0.file_id").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(out, "large_number").Raw)
}

func TestExcelBPSImageUploadResponseAndErrors(t *testing.T) {
	_, raw := excelBPSImageFixture(t)
	for _, field := range []string{"openai_file_id", "file_id", "id"} {
		upstream := &excelBPSImageUpstream{uploadJSON: fmt.Sprintf(`{"%s":"file-existing"}`, field)}
		id, err := newExcelBPSImageTestService(upstream).uploadExcelBPSImage(context.Background(), excelAccount(), "token", "account", "image/png", raw)
		require.NoError(t, err)
		require.Equal(t, "file-existing", id)
	}
	for _, upstream := range []*excelBPSImageUpstream{
		{uploadStatus: 403, uploadJSON: `{"error":"PRIVATE_IMAGE_BYTES token-secret"}`},
		{uploadStatus: 302},
		{uploadJSON: `{"id":"file/bad"}`},
		{uploadJSON: `{"id":null}`},
		{uploadJSON: `PRIVATE_IMAGE_BYTES`},
		{uploadError: errors.New("PRIVATE_IMAGE_BYTES token-secret")},
	} {
		svc := newExcelBPSImageTestService(upstream)
		_, err := svc.excelBPSImageFileID(context.Background(), excelAccount(), "token-secret", "account", "image/png", raw)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "PRIVATE_IMAGE_BYTES")
		require.NotContains(t, err.Error(), "token-secret")
		require.Empty(t, svc.excelBPSImages.entries)
	}
}

type excelBPSImagePersistentStub struct {
	GatewayCache
	entries map[string]excelBPSImageEntry
}

func (s *excelBPSImagePersistentStub) GetBasispointsImageFile(ctx context.Context, scope, digest string) (string, time.Time, error) {
	e := s.entries[scope+digest]
	return e.fileID, e.expiresAt, nil
}
func (s *excelBPSImagePersistentStub) SetBasispointsImageFile(ctx context.Context, scope, digest, id string, expires time.Time) error {
	s.entries[scope+digest] = excelBPSImageEntry{fileID: id, expiresAt: expires}
	return nil
}

func TestExcelBPSImagesAccountIsolationAndPersistentReuse(t *testing.T) {
	_, raw := excelBPSImageFixture(t)
	upstream := &excelBPSImageUpstream{}
	svc := newExcelBPSImageTestService(upstream)
	persistent := &excelBPSImagePersistentStub{entries: make(map[string]excelBPSImageEntry)}
	svc.cache = persistent
	for _, accountID := range []string{"a", "a", "b"} {
		_, err := svc.excelBPSImageFileID(context.Background(), excelAccount(), "token", accountID, "image/png", raw)
		require.NoError(t, err)
	}
	require.Equal(t, 2, upstream.uploads)
	restoredUpstream := &excelBPSImageUpstream{}
	restored := newExcelBPSImageTestService(restoredUpstream)
	restored.cache = persistent
	id, err := restored.excelBPSImageFileID(context.Background(), excelAccount(), "refreshed-token", "a", "image/png", raw)
	require.NoError(t, err)
	require.Equal(t, "file-upload-1", id)
	require.Empty(t, restoredUpstream.requests)
	other := excelAccount()
	other.ID++
	_, err = restored.excelBPSImageFileID(context.Background(), other, "token", "a", "image/png", raw)
	require.NoError(t, err)
	require.Equal(t, 1, restoredUpstream.uploads)
}

func TestExcelBPSImagesConcurrentDeduplicationAndCancellation(t *testing.T) {
	_, raw := excelBPSImageFixture(t)
	upstream := &excelBPSImageUpstream{delay: 25 * time.Millisecond}
	svc := newExcelBPSImageTestService(upstream)
	var group sync.WaitGroup
	errorsOut := make(chan error, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			id, err := svc.excelBPSImageFileID(context.Background(), excelAccount(), "token", "account", "image/png", raw)
			if err == nil && id != "file-upload-1" {
				err = fmt.Errorf("unexpected id %s", id)
			}
			errorsOut <- err
		}()
	}
	group.Wait()
	close(errorsOut)
	for err := range errorsOut {
		require.NoError(t, err)
	}
	require.Equal(t, 1, upstream.uploads)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.excelBPSImageFileID(ctx, excelAccount(), "token", "other", "image/png", raw)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, upstream.uploads)
}

func TestExcelBPSImagesMemoryCapacityAndExpiry(t *testing.T) {
	var cache excelBPSImageCache
	for i := 0; i < 520; i++ {
		cache.put(fmt.Sprint(i), excelBPSImageEntry{fileID: "file-test", expiresAt: time.Now().Add(time.Hour)})
	}
	require.Len(t, cache.entries, 512)
	cache.put("expired", excelBPSImageEntry{fileID: "file-old", expiresAt: time.Now().Add(-time.Hour)})
	_, ok := cache.get("expired")
	require.False(t, ok)
}

func TestExcelBPSImagesEscapedTypeAndOversizeInput(t *testing.T) {
	dataURL, _ := excelBPSImageFixture(t)
	body := excelBPSImageBody(t, map[string]any{"type": "input_image", "image_url": dataURL})
	escaped := bytes.ReplaceAll(body, []byte("input_image"), []byte(string(rune(92))+"u0069nput_image"))
	upstream := &excelBPSImageUpstream{}
	svc := newExcelBPSImageTestService(upstream)
	_, vision, err := svc.materializeExcelBPSImages(context.Background(), escaped, excelAccount(), "token", "account")
	require.NoError(t, err)
	require.True(t, vision)
	require.Equal(t, 1, upstream.uploads)
	oversized := "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, excelBPSMaxImageBytes+1))
	body = excelBPSImageBody(t, map[string]any{"type": "input_image", "image_url": oversized})
	_, _, err = svc.materializeExcelBPSImages(context.Background(), body, excelAccount(), "token", "account")
	require.Error(t, err)
	require.Equal(t, 1, upstream.uploads)
}

func TestExcelBPSImagesFileIDAndHTTPSForwardVisionHeader(t *testing.T) {
	for _, part := range []map[string]any{
		{"type": "input_image", "file_id": "file-existing"},
		{"type": "input_image", "image_url": "https://images.example/a.png"},
		{"type": "input_text", "text": "hello"},
	} {
		upstream := &excelBPSImageUpstream{}
		svc := newExcelBPSImageTestService(upstream)
		body := excelBPSImageBody(t, part)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
		_, err := svc.Forward(context.Background(), c, excelAccount(), body)
		require.NoError(t, err)
		require.Zero(t, upstream.uploads)
		require.Len(t, upstream.requests, 1)
		expected := ""
		if part["type"] == "input_image" {
			expected = "true"
		}
		require.Equal(t, expected, upstream.requests[0].Header.Get("Copilot-Vision-Request"))
	}
}

func TestExcelBPSImagesExpiredEntryRefreshes(t *testing.T) {
	_, raw := excelBPSImageFixture(t)
	upstream := &excelBPSImageUpstream{}
	svc := newExcelBPSImageTestService(upstream)
	_, err := svc.excelBPSImageFileID(context.Background(), excelAccount(), "token", "account", "image/png", raw)
	require.NoError(t, err)
	svc.excelBPSImages.mu.Lock()
	for key, value := range svc.excelBPSImages.entries {
		value.expiresAt = time.Now().Add(-time.Second)
		svc.excelBPSImages.entries[key] = value
	}
	svc.excelBPSImages.mu.Unlock()
	id, err := svc.excelBPSImageFileID(context.Background(), excelAccount(), "token", "account", "image/png", raw)
	require.NoError(t, err)
	require.Equal(t, "file-upload-2", id)
	require.Equal(t, 2, upstream.uploads)
}

func TestExcelBPSImageFailurePreventsForwardAndAllowsRetry(t *testing.T) {
	dataURL, _ := excelBPSImageFixture(t)
	upstream := &excelBPSImageUpstream{uploadStatus: 403, uploadJSON: `{"error":"PRIVATE_IMAGE_BYTES token-secret"}`}
	svc := newExcelBPSImageTestService(upstream)
	body := excelBPSImageBody(t, map[string]any{"type": "input_image", "image_url": dataURL})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	_, err := svc.Forward(context.Background(), c, excelAccount(), body)
	require.Error(t, err)
	require.Equal(t, 502, rec.Code)
	require.True(t, IsResponseCommitted(c))
	require.NotContains(t, rec.Body.String(), "PRIVATE_IMAGE_BYTES")
	require.NotContains(t, rec.Body.String(), "token-secret")
	require.Len(t, upstream.requests, 1)
	require.Empty(t, svc.excelBPSImages.entries)
	upstream.uploadStatus = 200
	upstream.uploadJSON = ""
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	_, err = svc.Forward(context.Background(), c, excelAccount(), body)
	require.NoError(t, err)
	require.Equal(t, 2, upstream.uploads)
	require.Len(t, upstream.requests, 3)
}

type excelBPSUnavailableCache struct{ GatewayCache }

func (s *excelBPSUnavailableCache) GetBasispointsImageFile(context.Context, string, string) (string, time.Time, error) {
	return "", time.Time{}, errors.New("cache unavailable")
}
func (s *excelBPSUnavailableCache) SetBasispointsImageFile(context.Context, string, string, string, time.Time) error {
	return errors.New("cache unavailable")
}

func TestExcelBPSImagesRedisOutageUsesMemory(t *testing.T) {
	_, raw := excelBPSImageFixture(t)
	upstream := &excelBPSImageUpstream{}
	svc := newExcelBPSImageTestService(upstream)
	svc.cache = &excelBPSUnavailableCache{}
	for i := 0; i < 2; i++ {
		id, err := svc.excelBPSImageFileID(context.Background(), excelAccount(), "token", "account", "image/png", raw)
		require.NoError(t, err)
		require.Equal(t, "file-upload-1", id)
	}
	require.Equal(t, 1, upstream.uploads)
}
