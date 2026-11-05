package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPoolManager_NormalizationAndMask(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn/v1/", 120)
	if pool.upstreamURL != "https://apihub.agnes-ai.cn" {
		t.Fatalf("expected upstream to be normalized to https://apihub.agnes-ai.cn, got %q", pool.upstreamURL)
	}

	keys := []string{
		"sk-agnes-1234567890abcdef",
		"sk-short",
	}
	pool.SetKeys(keys)

	list, _ := pool.Snapshot()
	if len(list) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(list))
	}

	if list[0].MaskedKey != "sk-a...cdef" {
		t.Fatalf("unexpected masked key: %s", list[0].MaskedKey)
	}
	if list[1].MaskedKey != "******" {
		t.Fatalf("unexpected masked key for short key: %s", list[1].MaskedKey)
	}
}

func TestPoolManager_RoundRobinAndCooldown(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn", 60)
	pool.SetKeys([]string{"key1", "key2", "key3"})

	k1, total := pool.PickNext()
	if total != 3 || k1.Key != "key1" {
		t.Fatalf("expected key1, got %v", k1)
	}
	k2, _ := pool.PickNext()
	if k2.Key != "key2" {
		t.Fatalf("expected key2, got %v", k2)
	}

	// 模拟 key2 触发 429
	pool.MarkCooldown(k2, "429_rate_limit", 1*time.Minute)

	// 下一次 PickNext 应该跳过处于冷却中的 key2，直接选 key3
	k3, _ := pool.PickNext()
	if k3.Key != "key3" {
		t.Fatalf("expected key3, got %v", k3)
	}
	// 再下一次应该选 key1（因为 key2 在冷却中）
	k1Again, _ := pool.PickNext()
	if k1Again.Key != "key1" {
		t.Fatalf("expected key1 again, got %v", k1Again)
	}
}

func TestNormalizeDNSServer(t *testing.T) {
	cases := map[string]string{
		"223.5.5.5":       "223.5.5.5:53",
		" 119.29.29.29 ":  "119.29.29.29:53",
		"8.8.8.8:5353":    "8.8.8.8:5353",
		"::1":             "[::1]:53",
		"":                "",
		"not-a-dns-entry": "",
	}
	for input, want := range cases {
		got, ok := normalizeDNSServer(input)
		if want == "" {
			if ok {
				t.Fatalf("expected %q to be rejected, got %q", input, got)
			}
			continue
		}
		if !ok || got != want {
			t.Fatalf("normalizeDNSServer(%q) = (%q, %v), want %q", input, got, ok, want)
		}
	}
}

func TestGatewayServer_InternalAPIAndHotReload(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn", 120)
	pool.SetKeys([]string{"init-key1"})
	server := NewGatewayServer(pool, 8080, 2)

	// 1. 验证外部局域网 IP 调用管理接口被拒绝 403
	extReq := httptest.NewRequest(http.MethodGet, "/_gateway/status", nil)
	extReq.RemoteAddr = "192.168.1.100:54321"
	extRR := httptest.NewRecorder()
	server.ServeHTTP(extRR, extReq)
	if extRR.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for external IP, got %d", extRR.Code)
	}

	// 2. 测试本地回环 GET /_gateway/status
	req := httptest.NewRequest(http.MethodGet, "/_gateway/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var statusResp struct {
		TotalKeys int `json:"total_keys"`
	}
	json.NewDecoder(rr.Body).Decode(&statusResp)
	if statusResp.TotalKeys != 1 {
		t.Fatalf("expected 1 total key, got %d", statusResp.TotalKeys)
	}

	// 3. 测试本地回环 POST /_gateway/keys 热更新号池
	updateBody := bytes.NewBufferString("new-key1\nnew-key2\nnew-key3")
	postReq := httptest.NewRequest(http.MethodPost, "/_gateway/keys", updateBody)
	postReq.RemoteAddr = "127.0.0.1:12345"
	postRR := httptest.NewRecorder()
	server.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on key reload, got %d", postRR.Code)
	}

	// 重新查看状态，确认已热更新为 3 个 Key
	list, _ := pool.Snapshot()
	if len(list) != 3 {
		t.Fatalf("expected 3 keys after hot-reload, got %d", len(list))
	}
}

func TestGatewayServer_ForwardAndRetryOn429(t *testing.T) {
	// 启动一个模拟的上游 Agnes 服务器：第 1 个 Key 返回 429，第 2 个 Key 成功
	requestCount := 0
	upstreamMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		if auth == "Bearer fail-key" {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"requests"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
	}))
	defer upstreamMock.Close()

	pool := NewPoolManager(upstreamMock.URL, 60)
	pool.SetKeys([]string{"fail-key", "success-key"})
	server := NewGatewayServer(pool, 8080, 2)

	// 发起模拟请求
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-6-astra"}`))
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 after retry, got %d", rr.Code)
	}
	if requestCount != 2 {
		t.Fatalf("expected exactly 2 requests (1 fail + 1 retry), got %d", requestCount)
	}
}
