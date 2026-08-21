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
	batchCalls := 0
	downloadToken := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/health-check":
			_, _ = w.Write([]byte(`{"ok":true,"need_reclaim":1,"healthy":0}`))
		case "/api/redeem/reclaim/batch-cards":
			batchCalls++
			if batchCalls == 1 {
				_, _ = w.Write([]byte(`{"ok":true,"queued":1,"already_running":1,"cards":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"RCL-ABCD","tasks":[{"order_no":"ord-401","resource_uid":"acct-1","status":"done","download_token":"tok-401"}]}]}`))
		case "/api/redeem/orders/ord-401/download":
			downloadToken = r.URL.Query().Get("token")
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
	if downloadToken != "tok-401" {
		t.Fatalf("download token = %q", downloadToken)
	}
}
