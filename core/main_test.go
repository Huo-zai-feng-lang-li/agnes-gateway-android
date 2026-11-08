package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
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

	list, _, _, available, _, _ := pool.Snapshot()
	if len(list) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(list))
	}
	if available != 2 {
		t.Fatalf("expected 2 available keys, got %d", available)
	}

	if list[0].MaskedKey != "...cdef" {
		t.Fatalf("unexpected masked key: %s", list[0].MaskedKey)
	}
	if list[1].MaskedKey != "******" {
		t.Fatalf("unexpected masked key for short key: %s", list[1].MaskedKey)
	}
}

func TestPoolManager_AliasParsing(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn", 60)
	pool.SetKeys([]string{
		"sk-1234567890abcdef#主力号",
		"sk-9876543210fedcba----备用号",
		"sk-abcdef1234567890 应急号",
		"sk-plainkey12345678",
	})

	list, _, _, _, _, _ := pool.Snapshot()
	if len(list) != 4 {
		t.Fatalf("expected 4 keys, got %d", len(list))
	}

	if list[0].Alias != "主力号" || list[0].DisplayName != "主力号(...cdef)" {
		t.Fatalf("alias 1 mismatch: %s / %s", list[0].Alias, list[0].DisplayName)
	}
	if list[1].Alias != "备用号" || list[1].DisplayName != "备用号(...dcba)" {
		t.Fatalf("alias 2 mismatch: %s / %s", list[1].Alias, list[1].DisplayName)
	}
	if list[2].Alias != "应急号" || list[2].DisplayName != "应急号(...7890)" {
		t.Fatalf("alias 3 mismatch: %s / %s", list[2].Alias, list[2].DisplayName)
	}
	if list[3].Alias != "" || list[3].DisplayName != "...5678" {
		t.Fatalf("alias 4 mismatch: %s / %s", list[3].Alias, list[3].DisplayName)
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

func TestPoolManager_RemoveKeyOn401(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn", 60)
	pool.SetKeys([]string{"key-valid-1", "key-banned-2", "key-valid-3"})

	removed := pool.RemoveKey("key-banned-2", "401_unauthorized")
	if !removed {
		t.Fatalf("expected key-banned-2 to be removed")
	}

	list, _, _, available, _, banned := pool.Snapshot()
	if len(list) != 2 {
		t.Fatalf("expected 2 remaining keys, got %d", len(list))
	}
	if available != 2 {
		t.Fatalf("expected 2 available keys, got %d", available)
	}
	if banned != 1 {
		t.Fatalf("expected 1 banned key count, got %d", banned)
	}

	for _, ks := range list {
		if ks.Key == "key-banned-2" {
			t.Fatalf("banned key still present in pool!")
		}
	}

	// 重新加入已被封禁的 Key，验证 banned 计数回退
	pool.SetKeys([]string{"key-valid-1", "key-banned-2"})
	_, _, _, _, _, bannedAfterReadd := pool.Snapshot()
	if bannedAfterReadd != 0 {
		t.Fatalf("expected 0 banned keys after re-adding, got %d", bannedAfterReadd)
	}
}

func TestFastDNSResolver_Cache(t *testing.T) {
	resolver := NewFastDNSResolver(nil)
	ctx := context.Background()

	// 模拟直接解析 IP
	ips, err := resolver.ResolveHost(ctx, "127.0.0.1")
	if err != nil || len(ips) == 0 || !ips[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("failed to resolve IP directly: %v, %v", ips, err)
	}

	// 手动塞入缓存测试命中
	testIP := net.ParseIP("1.2.3.4")
	resolver.cache.Store("example.com", dnsCacheEntry{
		ips:     []net.IP{testIP},
		expires: time.Now().Add(10 * time.Minute),
	})

	cachedIPs, err := resolver.ResolveHost(ctx, "example.com")
	if err != nil || len(cachedIPs) == 0 || !cachedIPs[0].Equal(testIP) {
		t.Fatalf("expected cached IP 1.2.3.4, got %v, err=%v", cachedIPs, err)
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
		TotalKeys     int    `json:"total_keys"`
		AvailableKeys int    `json:"available_keys"`
		LastUsedKey   string `json:"last_used_key"`
	}
	json.NewDecoder(rr.Body).Decode(&statusResp)
	if statusResp.TotalKeys != 1 || statusResp.AvailableKeys != 1 {
		t.Fatalf("expected 1 total & available key, got %v", statusResp)
	}

	// 3. 测试本地回环 POST /_gateway/keys 热更新号池
	updateBody := bytes.NewBufferString("new-key1#主力\nnew-key2#备用\nnew-key3")
	postReq := httptest.NewRequest(http.MethodPost, "/_gateway/keys", updateBody)
	postReq.RemoteAddr = "127.0.0.1:12345"
	postRR := httptest.NewRecorder()
	server.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on key reload, got %d", postRR.Code)
	}

	// 重新查看状态，确认已热更新为 3 个 Key
	list, _, _, _, _, _ := pool.Snapshot()
	if len(list) != 3 {
		t.Fatalf("expected 3 keys after hot-reload, got %d", len(list))
	}
}

func TestGatewayServer_RemoveOn401AndRetry(t *testing.T) {
	// 模拟上游：第一个 Key 返回 401 Unauthorized，第二个 Key 返回 200 OK
	requestCount := 0
	upstreamMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		auth := r.Header.Get("Authorization")
		if auth == "Bearer banned-key" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"Invalid key or banned","type":"authentication_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello from Agnes\"}}]}\n\n"))
	}))
	defer upstreamMock.Close()

	pool := NewPoolManager(upstreamMock.URL, 60)
	pool.SetKeys([]string{"banned-key#被封Key", "valid-key#正常Key"})
	server := NewGatewayServer(pool, 8080, 2)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"agnes-2.5-flash"}`))
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 after retry on 401, got %d", rr.Code)
	}

	// 验证 banned-key 已被从号池中剔除
	list, _, _, available, _, banned := pool.Snapshot()
	if len(list) != 1 {
		t.Fatalf("expected 1 remaining key in pool, got %d", len(list))
	}
	if list[0].Key != "valid-key" {
		t.Fatalf("expected remaining key to be valid-key, got %s", list[0].Key)
	}
	if available != 1 {
		t.Fatalf("expected 1 available key, got %d", available)
	}
	if banned != 1 {
		t.Fatalf("expected 1 banned key count, got %d", banned)
	}
}
