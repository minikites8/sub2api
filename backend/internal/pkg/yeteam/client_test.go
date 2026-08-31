package yeteam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeAccountPayload(t *testing.T) {
	data, err := NormalizeAccountPayload([]byte(`{"data":{"accounts":[{"name":"user RCL-ABCD","credentials":{"access_token":"token"}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["type"] != "sub2api-data" {
		t.Fatalf("type = %v", payload["type"])
	}
	matched, err := FindAccountCredentials(data, "user RCL-ABCD")
	if err != nil {
		t.Fatal(err)
	}
	if matched.Credentials["access_token"] != "token" {
		t.Fatalf("credentials = %#v", matched.Credentials)
	}
}

func TestClientRuntimeEnabledToggle(t *testing.T) {
	client := NewClient(Config{Enabled: false, AutoRefresh401: true})
	if client.Enabled() || client.AutoRefresh401Enabled() {
		t.Fatal("client should start disabled")
	}
	client.SetEnabled(true)
	if !client.Enabled() || !client.AutoRefresh401Enabled() {
		t.Fatal("client should be enabled after runtime update")
	}
	client.SetEnabled(false)
	if client.Enabled() || client.AutoRefresh401Enabled() {
		t.Fatal("client should be disabled after runtime update")
	}
}

func TestClientRedeemRequestAndDownload(t *testing.T) {
	var paths []string
	var orderStatusAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/preview":
			_, _ = w.Write([]byte(`{"data":{"action":"redeem_remaining"}}`))
		case "/api/redeem/orders":
			_, _ = w.Write([]byte(`{"data":{"order_no":"ord-1","status":"completed","download_token":"tok"}}`))
		case "/api/redeem/orders/ord-1":
			orderStatusAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"data":{"status":"completed"}}`))
		case "/api/redeem/orders/ord-1/download":
			_, _ = w.Write([]byte(`{"accounts":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewClient(Config{Enabled: true, BaseURL: server.URL, Timeout: time.Second, PollInterval: time.Millisecond})
	if _, err := client.Preview(context.Background(), PreviewRequest{CardCode: "RCL-ABCD"}); err != nil {
		t.Fatal(err)
	}
	order, err := client.Redeem(context.Background(), RedeemRequest{CardCode: "RCL-ABCD", Action: "redeem_remaining"})
	if err != nil || order.OrderNo != "ord-1" {
		t.Fatalf("order=%#v err=%v", order, err)
	}
	finalOrder, err := client.PollUntilDone(context.Background(), order.OrderNo, order.DownloadToken)
	if err != nil {
		t.Fatal(err)
	}
	if finalOrder.OrderNo != "ord-1" || finalOrder.DownloadToken != "tok" {
		t.Fatalf("final order = %#v", finalOrder)
	}
	if orderStatusAuth != "Bearer tok" {
		t.Fatalf("order status authorization = %q", orderStatusAuth)
	}
	if _, err := client.Download(context.Background(), order.OrderNo, order.DownloadToken); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestReclaim401PackagesUsesTaskDownloadTokens(t *testing.T) {
	type reclaimBody struct {
		CardCodes []string `json:"card_codes"`
		Mode      *string  `json:"mode"`
		QueryOnly *bool    `json:"query_only"`
	}
	healthCalls := 0
	batchCalls := 0
	batchRequests := make([]reclaimBody, 0, 2)
	var batchDownloadRequest BatchDownloadRequest
	batchDownloadCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/health-check":
			healthCalls++
			w.WriteHeader(http.StatusInternalServerError)
		case "/api/redeem/reclaim/batch-cards":
			batchCalls++
			var body reclaimBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			batchRequests = append(batchRequests, body)
			_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"RCL-ABCD","card_status":"ok","tasks":[{"order_no":"ord-401","resource_uid":"acct-1","status":"done","download_token":"tok-401","no_action":true,"message":"凭据正常，无需找回"}]}]}`))
		case "/api/redeem/batch-download":
			batchDownloadCalls++
			if err := json.NewDecoder(r.Body).Decode(&batchDownloadRequest); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"new-token"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewClient(Config{Enabled: true, BaseURL: server.URL, Timeout: time.Second, PollInterval: time.Millisecond})
	packages, err := client.Reclaim401Packages(context.Background(), "RCL-ABCD")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || !json.Valid(packages[0]) {
		t.Fatalf("packages = %d", len(packages))
	}
	if batchDownloadCalls != 1 {
		t.Fatalf("batch download calls = %d", batchDownloadCalls)
	}
	if batchDownloadRequest.ExportMode != "multi_account_json" || len(batchDownloadRequest.Items) != 1 ||
		batchDownloadRequest.Items[0].OrderNo != "ord-401" || batchDownloadRequest.Items[0].DownloadToken != "tok-401" ||
		batchDownloadRequest.Summary == nil || len(batchDownloadRequest.Summary) != 0 {
		t.Fatalf("batch download request = %#v", batchDownloadRequest)
	}
	if healthCalls != 0 || batchCalls != 1 {
		t.Fatalf("health calls = %d, batch calls = %d", healthCalls, batchCalls)
	}
	if len(batchRequests) != 1 {
		t.Fatalf("batch requests = %d", len(batchRequests))
	}
	if len(batchRequests[0].CardCodes) != 1 || batchRequests[0].CardCodes[0] != "RCL-ABCD" ||
		batchRequests[0].Mode == nil || *batchRequests[0].Mode != "401" ||
		batchRequests[0].QueryOnly == nil || *batchRequests[0].QueryOnly {
		t.Fatalf("submit request = %#v", batchRequests[0])
	}
}

func TestBatchReclaimResultAcceptsCardCodesAliasAndCamelCaseTaskFields(t *testing.T) {
	var result BatchReclaimResult
	err := json.Unmarshal([]byte(`{"ok":true,"done":1,"card_codes":{"TEAM-TEST":{"card_status":"ok","tasks":[{"orderNo":"ord-1","downloadToken":"tok-1","status":"completed","resourceUid":"acct-1"}]}}}`), &result)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cards) != 1 || len(result.AllTasks) != 1 {
		t.Fatalf("cards=%#v tasks=%#v", result.Cards, result.AllTasks)
	}
	task := result.AllTasks[0]
	if task.CardCode != "TEAM-TEST" || task.OrderNo != "ord-1" || task.DownloadToken != "tok-1" || task.Status != "completed" {
		t.Fatalf("task=%#v", task)
	}
}

func TestCollectBatchDownloadItemsKeepsInitialTokenWhenPollOmitsIt(t *testing.T) {
	initial := BatchReclaimResult{AllTasks: []ReclaimTask{{OrderNo: "ord-1", DownloadToken: "tok-1", Status: "done"}}}
	final := BatchReclaimResult{AllTasks: []ReclaimTask{{OrderNo: "ord-1", Status: "completed"}}}
	items := collectBatchDownloadItems(initial, final)
	if len(items) != 1 || items[0].OrderNo != "ord-1" || items[0].DownloadToken != "tok-1" {
		t.Fatalf("items = %#v", items)
	}
}

func TestReclaim401PackagesQueriesTerminalResponseUntilTokenIsAvailable(t *testing.T) {
	batchCalls := 0
	batchDownloadCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/batch-cards":
			batchCalls++
			switch batchCalls {
			case 1:
				_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"TEAM-TEST","tasks":[{"order_no":"ord-1","status":"done"}]}]}`))
				return
			case 2:
				_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":0,"cards":[{"card_code":"TEAM-TEST","card_status":"ok","tasks":[]}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"TEAM-TEST","tasks":[{"order_no":"ord-1","status":"done","download_token":"tok-1"}]}]}`))
		case "/api/redeem/batch-download":
			batchDownloadCalls++
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"token"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(Config{Enabled: true, BaseURL: server.URL, Timeout: time.Second, PollInterval: time.Millisecond})
	packages, err := client.Reclaim401Packages(context.Background(), "TEAM-TEST")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || batchCalls != 3 || batchDownloadCalls != 1 {
		t.Fatalf("packages=%d batch_calls=%d batch_download_calls=%d", len(packages), batchCalls, batchDownloadCalls)
	}
}

func TestReclaim401PackagesStopsOnFailedTask(t *testing.T) {
	batchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batchCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":0,"failed":1,"cards":[{"card_code":"TEAM-TEST","tasks":[{"order_no":"ord-1","status":"failed","message":"upstream rejected"}]}]}`))
	}))
	defer server.Close()
	client := NewClient(Config{Enabled: true, BaseURL: server.URL, Timeout: time.Second, PollInterval: time.Millisecond, MaxPollDuration: time.Second})
	_, err := client.Reclaim401Packages(context.Background(), "TEAM-TEST")
	if err == nil || err.Error() != "upstream rejected" {
		t.Fatalf("err = %v", err)
	}
	if batchCalls != 1 {
		t.Fatalf("batch calls = %d", batchCalls)
	}
}

func TestReclaim401PackagesDownloadsNoActionWithoutDownloadToken(t *testing.T) {
	batchCalls := 0
	downloadCalls := 0
	var downloadRequest BatchDownloadRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/batch-cards":
			batchCalls++
			_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"TEAM-TEST","tasks":[{"order_no":"ord-1","status":"done","no_action":true,"message":"credential healthy"}]}]}`))
		case "/api/redeem/batch-download":
			downloadCalls++
			if err := json.NewDecoder(r.Body).Decode(&downloadRequest); err != nil {
				t.Fatalf("decode download request: %v", err)
			}
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"new-token"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewClient(Config{Enabled: true, BaseURL: server.URL, Timeout: time.Second, PollInterval: time.Millisecond, MaxPollDuration: time.Second})
	packages, err := client.Reclaim401Packages(context.Background(), "TEAM-TEST")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if batchCalls != 1 {
		t.Fatalf("batch calls = %d, want 1", batchCalls)
	}
	if downloadCalls != 1 || len(downloadRequest.Items) != 1 || downloadRequest.Items[0].OrderNo != "ord-1" || downloadRequest.Items[0].DownloadToken != "" {
		t.Fatalf("download calls=%d request=%#v", downloadCalls, downloadRequest)
	}
	if len(packages) != 1 {
		t.Fatalf("packages = %d, want 1", len(packages))
	}
}

func TestReclaim401PackagesStopsPollingWhenNoActionBecomesTerminal(t *testing.T) {
	batchCalls := 0
	downloadCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/batch-cards":
			batchCalls++
			if batchCalls == 1 {
				_, _ = w.Write([]byte(`{"ok":true,"queued":1,"already_running":0,"cards":[{"card_code":"TEAM-TEST","tasks":[{"status":"pending"}]}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"TEAM-TEST","tasks":[{"order_no":"ord-1","status":"done","no_action":true}]}]}`))
		case "/api/redeem/batch-download":
			downloadCalls++
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"new-token"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewClient(Config{Enabled: true, BaseURL: server.URL, Timeout: time.Second, PollInterval: time.Millisecond, MaxPollDuration: time.Second})
	packages, err := client.Reclaim401Packages(context.Background(), "TEAM-TEST")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(packages) != 1 || batchCalls != 2 || downloadCalls != 1 {
		t.Fatalf("packages=%d batch_calls=%d download_calls=%d", len(packages), batchCalls, downloadCalls)
	}
}
