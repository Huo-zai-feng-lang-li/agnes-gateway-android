package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
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
	bodyBytes := rr.Body.Bytes()
	if bytes.Contains(bodyBytes, []byte("init-key1")) {
		t.Fatalf("management API must not leak plaintext key, got: %s", bodyBytes)
	}
	json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&statusResp)
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

// 全部 Key 处于冷却时必须立即返回 nil（由上层快速 503），不得再挑一个冷却中的 Key 去撞上游
func TestPoolManager_AllCoolingReturnsNil(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn", 60)
	pool.SetKeys([]string{"k1", "k2"})

	k1, _ := pool.PickNext()
	k2, _ := pool.PickNext()
	pool.MarkCooldown(k1, "429", 1*time.Minute)
	pool.MarkCooldown(k2, "429", 1*time.Minute)

	got, total := pool.PickNext()
	if got != nil {
		t.Fatalf("expected nil when all keys cooling, got %s", got.DisplayName)
	}
	if total != 2 {
		t.Fatalf("expected total=2 to distinguish from empty pool, got %d", total)
	}

	// 冷却到期后必须重新可用
	time.Sleep(60 * time.Millisecond)
	pool.mu.Lock()
	for _, ks := range pool.keys {
		ks.CooldownTill = time.Now().Add(-time.Second)
	}
	pool.mu.Unlock()
	if again, _ := pool.PickNext(); again == nil {
		t.Fatalf("expected key available after cooldown expiry")
	}
}

// 空池必须返回 (nil, 0)，与「全冷却」(nil, total>0) 区分
func TestPoolManager_EmptyPoolReturnsZeroTotal(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn", 60)
	pool.SetKeys(nil)
	ks, total := pool.PickNext()
	if ks != nil || total != 0 {
		t.Fatalf("expected (nil,0) for empty pool, got (%v,%d)", ks, total)
	}
}

// 并发拨号：首个 IP 为黑洞时，必须由后续 IP 快速胜出，而非串行等到首 IP 超时
func TestDialIPsFallback_RacesPastBlackhole(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	// 10.255.255.1 为典型黑洞地址（SYN 无响应），排在 v4 首位
	ips := []net.IP{net.ParseIP("10.255.255.1"), net.ParseIP("127.0.0.1")}
	d := &net.Dialer{Timeout: 800 * time.Millisecond}

	start := time.Now()
	conn, err := dialIPsFallback(context.Background(), d, "tcp", ips, port)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected race to succeed via 127.0.0.1, got %v", err)
	}
	conn.Close()

	// 串行实现需等首 IP 满 800ms 超时；并发应远小于该值
	if elapsed > 400*time.Millisecond {
		t.Fatalf("dial too slow (%v), race likely not concurrent", elapsed)
	}
}

// 热更新号池与 PickNext 并发时不得触发数据竞争（go test -race 下验证）
func TestPoolManager_HotReloadConcurrentWithPick(t *testing.T) {
	pool := NewPoolManager("https://apihub.agnes-ai.cn", 60)
	pool.SetKeys([]string{"a1#一号", "a2#二号"})

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				pool.SetKeys([]string{"a1#一号改名", "a2#二号", "a3#三号"})
			}
		}
	}()

	for i := 0; i < 200; i++ {
		ks, _ := pool.PickNext()
		if ks != nil {
			_ = ks.DisplayName
		}
		pool.Snapshot()
	}
	close(stop)
	wg.Wait()
}

// 客户端主动断开（ctx 取消）不得把 Key 打入冷却，否则会误伤整个号池
func TestForwardRequest_ClientCancelDoesNotCooldown(t *testing.T) {
	// release 用于在测试收尾时释放卡住的 handler，避免 httptest.Server.Close 死等
	release := make(chan struct{})
	upstreamMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		upstreamMock.Close()
	}()

	pool := NewPoolManager(upstreamMock.URL, 60)
	pool.SetKeys([]string{"k1#主力", "k2#备用"})
	server := NewGatewayServer(pool, 8080, 2)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"m"}`)).WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		server.ServeHTTP(rr, req)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("handler did not return after client cancel")
	}

	// 两个 Key 都必须仍可用（未进入冷却）
	_, _, _, available, cooldown, _ := pool.Snapshot()
	if cooldown != 0 {
		t.Fatalf("client cancel must not cooldown keys, got cooldown=%d", cooldown)
	}
	if available != 2 {
		t.Fatalf("expected 2 available keys after client cancel, got %d", available)
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
