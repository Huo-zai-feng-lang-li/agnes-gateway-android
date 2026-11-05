package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// KeyState 记录单个 Key 的运行时健康状态
type KeyState struct {
	Key          string    `json:"key"`
	MaskedKey    string    `json:"masked_key"`
	CooldownTill time.Time `json:"cooldown_till"`
	FailureCount int64     `json:"failure_count"`
	SuccessCount int64     `json:"success_count"`
	LastError    string    `json:"last_error"`
	Disabled     bool      `json:"disabled"`
}

// PoolManager 负责多 Key 轮询与智能退避调度
type PoolManager struct {
	mu           sync.RWMutex
	keys         []*KeyState
	currentIndex uint64
	upstreamURL  string
	cooldownSec  int
}

func maskKey(k string) string {
	k = strings.TrimSpace(k)
	if len(k) <= 8 {
		return "******"
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func NewPoolManager(upstream string, cooldownSec int) *PoolManager {
	if upstream == "" {
		upstream = "https://agnes-ai.cn"
	}
	upstream = strings.TrimSpace(upstream)
	upstream = strings.TrimSuffix(upstream, "/")
	upstream = strings.TrimSuffix(upstream, "/v1")
	upstream = strings.TrimSuffix(upstream, "/")
	if cooldownSec <= 0 {
		cooldownSec = 120
	}
	return &PoolManager{
		upstreamURL: upstream,
		cooldownSec: cooldownSec,
	}
}

func (p *PoolManager) SetKeys(rawKeys []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	keyMap := make(map[string]*KeyState)
	for _, old := range p.keys {
		keyMap[old.Key] = old
	}

	var newKeys []*KeyState
	seen := make(map[string]bool)
	for _, k := range rawKeys {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		if old, exists := keyMap[k]; exists {
			newKeys = append(newKeys, old)
		} else {
			newKeys = append(newKeys, &KeyState{
				Key:       k,
				MaskedKey: maskKey(k),
			})
		}
	}
	p.keys = newKeys
	log.Printf("[号池] 已加载 %d 个有效 Key", len(p.keys))
}

// PickNext 轮询挑选一个当前未处于冷却中的 Key
func (p *PoolManager) PickNext() (*KeyState, int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := len(p.keys)
	if total == 0 {
		return nil, 0
	}

	now := time.Now()
	// 最多轮询一整圈寻找可用 Key
	for i := 0; i < total; i++ {
		idx := int(atomic.AddUint64(&p.currentIndex, 1)-1) % total
		ks := p.keys[idx]
		if ks.Disabled {
			continue
		}
		if now.Before(ks.CooldownTill) {
			continue
		}
		return ks, total
	}

	// 若所有 Key 都在冷却中，挑选最早解冻的那一个
	var earliest *KeyState
	for _, ks := range p.keys {
		if ks.Disabled {
			continue
		}
		if earliest == nil || ks.CooldownTill.Before(earliest.CooldownTill) {
			earliest = ks
		}
	}
	return earliest, total
}

func (p *PoolManager) MarkSuccess(ks *KeyState) {
	atomic.AddInt64(&ks.SuccessCount, 1)
}

func (p *PoolManager) MarkCooldown(ks *KeyState, errReason string, duration time.Duration) {
	if duration <= 0 {
		duration = time.Duration(p.cooldownSec) * time.Second
	}
	atomic.AddInt64(&ks.FailureCount, 1)
	p.mu.Lock()
	ks.CooldownTill = time.Now().Add(duration)
	ks.LastError = errReason
	p.mu.Unlock()
	log.Printf("[号池退避] Key %s 触发限流/错误 (%s)，冷却 %v 至 %s", ks.MaskedKey, errReason, duration, ks.CooldownTill.Format("15:04:05"))
}

func (p *PoolManager) MarkDisabled(ks *KeyState, errReason string) {
	atomic.AddInt64(&ks.FailureCount, 1)
	p.mu.Lock()
	ks.Disabled = true
	ks.LastError = errReason
	p.mu.Unlock()
	log.Printf("[号池禁用] Key %s 永久失效/无效 (%s)，已禁用", ks.MaskedKey, errReason)
}

func (p *PoolManager) Snapshot() ([]KeyState, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	list := make([]KeyState, len(p.keys))
	for i, ks := range p.keys {
		list[i] = *ks
	}
	return list, p.upstreamURL
}

// customResolver 供上游 http.Transport 复用；nil 表示使用系统默认解析器
var customResolver *net.Resolver

// normalizeDNSServer 将裸 IP / host:port 归一化成 host:port 形式
func normalizeDNSServer(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if net.ParseIP(raw) != nil {
		return net.JoinHostPort(raw, "53"), true
	}
	if host, port, err := net.SplitHostPort(raw); err == nil && host != "" && port != "" {
		return raw, true
	}
	return "", false
}

// newResolver 构造基于指定 DNS 服务器的纯 Go 解析器
func newResolver(servers []string) *net.Resolver {
	if len(servers) == 0 {
		return nil
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 3 * time.Second}
			var lastErr error
			for _, server := range servers {
				conn, err := dialer.DialContext(ctx, network, server)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("no dns server available")
			}
			return nil, lastErr
		},
	}
}

// probeDNSServers 并行探测候选 DNS 服务器是否可用，按原顺序返回可用项
func probeDNSServers(servers []string, host string) []string {
	if len(servers) == 0 || host == "" || net.ParseIP(host) != nil {
		return servers
	}
	type probeResult struct {
		idx int
		ok  bool
	}
	ch := make(chan probeResult, len(servers))
	for i, server := range servers {
		go func(i int, server string) {
			r := newResolver([]string{server})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := r.LookupHost(ctx, host)
			ch <- probeResult{idx: i, ok: err == nil}
		}(i, server)
	}
	okSet := make(map[int]bool, len(servers))
	for range servers {
		r := <-ch
		if r.ok {
			okSet[r.idx] = true
		}
	}
	var ordered []string
	for i, server := range servers {
		if okSet[i] {
			ordered = append(ordered, server)
		}
	}
	return ordered
}

// GatewayServer 反代网关
type GatewayServer struct {
	pool       *PoolManager
	client     *http.Client
	listenAddr string
	maxRetries int
}

func NewGatewayServer(pool *PoolManager, port int, maxRetries int) *GatewayServer {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	// 上游长连接复用池（HTTP/2 + Keep-Alive + 自定义 DNS 解析）
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			Resolver:  customResolver,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}

	return &GatewayServer{
		pool: pool,
		client: &http.Client{
			Transport: transport,
			Timeout:   0, // 流式响应不能设全局超时
		},
		listenAddr: fmt.Sprintf("0.0.0.0:%d", port),
		maxRetries: maxRetries,
	}
}

func (s *GatewayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 跨域支持 (CORS)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 管理接口 (仅限本机回环访问，杜绝局域网未授权修改)
	if strings.HasPrefix(r.URL.Path, "/_gateway/") {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if host != "127.0.0.1" && host != "::1" && host != "localhost" {
			http.Error(w, `{"error":{"message":"Forbidden: 管理接口仅限本机内部调用","type":"forbidden"}}`, http.StatusForbidden)
			return
		}
		s.handleInternalAPI(w, r)
		return
	}

	// 根路径健康探针
	if r.URL.Path == "/" || r.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "running",
			"service":  "agnes-gateway-android",
			"upstream": s.pool.upstreamURL,
			"time":     time.Now().Format(time.RFC3339),
		})
		return
	}

	// 核心业务反代转发 (OpenAI / v1 / responses 协议)
	s.forwardRequest(w, r)
}

func (s *GatewayServer) handleInternalAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/_gateway/status":
		list, upstream := s.pool.Snapshot()
		available := 0
		now := time.Now()
		for _, k := range list {
			if !k.Disabled && now.After(k.CooldownTill) {
				available++
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]any{
			"upstream":       upstream,
			"total_keys":     len(list),
			"available_keys": available,
			"keys":           list,
		})

	case "/_gateway/keys":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Read body failed", http.StatusBadRequest)
			return
		}
		var req struct {
			Keys []string `json:"keys"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			// 兼容纯换行文本格式
			lines := strings.Split(string(body), "\n")
			s.pool.SetKeys(lines)
		} else {
			s.pool.SetKeys(req.Keys)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))

	default:
		http.NotFound(w, r)
	}
}

func (s *GatewayServer) forwardRequest(w http.ResponseWriter, r *http.Request) {
	// 读取请求体（为了重试机制，必须全量缓存请求体）
	var reqBody []byte
	if r.Body != nil {
		var err error
		reqBody, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "读取请求体失败: "+err.Error(), http.StatusBadRequest)
			return
		}
		r.Body.Close()
	}

	var lastStatusCode int
	var lastErrBody []byte

	for attempt := 0; attempt < s.maxRetries; attempt++ {
		keyState, totalKeys := s.pool.PickNext()
		if keyState == nil {
			http.Error(w, `{"error":{"message":"Agnes 号池为空或全部处于冷却中，请稍后重试","type":"gateway_pool_empty"}}`, http.StatusServiceUnavailable)
			return
		}

		targetURL := s.pool.upstreamURL + r.URL.Path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(reqBody))
		if err != nil {
			http.Error(w, "创建上游请求失败: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 复制原有请求头
		for k, v := range r.Header {
			// 跳过 Hop-by-hop 头
			if strings.EqualFold(k, "Connection") || strings.EqualFold(k, "Keep-Alive") || strings.EqualFold(k, "Host") {
				continue
			}
			outReq.Header[k] = v
		}

		// 注入号池中被轮询选中的 Agnes Key
		outReq.Header.Set("Authorization", "Bearer "+keyState.Key)

		// 发起上游请求
		resp, reqErr := s.client.Do(outReq)
		if reqErr != nil {
			log.Printf("[转发失败] Key %s 错误 (尝试 %d/%d): %v", keyState.MaskedKey, attempt+1, s.maxRetries, reqErr)
			s.pool.MarkCooldown(keyState, reqErr.Error(), 30*time.Second)
			if totalKeys > 1 {
				continue
			}
			http.Error(w, fmt.Sprintf(`{"error":{"message":"连接 Agnes 上游失败: %v","type":"upstream_connect_error"}}`, reqErr), http.StatusBadGateway)
			return
		}

		// 针对 429 速率限制退避重试 (30 秒灵敏冷却)
		if resp.StatusCode == http.StatusTooManyRequests {
			errBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastStatusCode = resp.StatusCode
			lastErrBody = errBytes
			s.pool.MarkCooldown(keyState, "429_rate_limit", 30*time.Second)
			log.Printf("[429 限速] Key %s 触发 429，立即切换下一个 Key 重试 (尝试 %d/%d)", keyState.MaskedKey, attempt+1, s.maxRetries)
			if totalKeys > 1 {
				continue
			}
			s.writeUpstreamError(w, resp.StatusCode, errBytes)
			return
		}

		// 针对 401 凭证无效（失效）标记禁用
		if resp.StatusCode == http.StatusUnauthorized {
			errBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastStatusCode = resp.StatusCode
			lastErrBody = errBytes
			s.pool.MarkDisabled(keyState, "401_unauthorized")
			log.Printf("[401 凭证无效] Key %s 已被上游拒绝，自动禁用", keyState.MaskedKey)
			if totalKeys > 1 {
				continue
			}
			s.writeUpstreamError(w, resp.StatusCode, errBytes)
			return
		}

		// 针对 5xx 临时错误重试 (15 秒冷却)
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			errBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastStatusCode = resp.StatusCode
			lastErrBody = errBytes
			s.pool.MarkCooldown(keyState, fmt.Sprintf("http_%d", resp.StatusCode), 15*time.Second)
			if totalKeys > 1 {
				continue
			}
			s.writeUpstreamError(w, resp.StatusCode, errBytes)
			return
		}

		// 成功响应！标记 Key 健康并流式返回
		s.pool.MarkSuccess(keyState)
		defer resp.Body.Close()

		// 复制响应头，清理 Hop-by-hop 头与 Content-Length（流式传输使用 chunked）
		isSSE := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
		for k, vv := range resp.Header {
			if strings.EqualFold(k, "Connection") || strings.EqualFold(k, "Keep-Alive") ||
				strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Upgrade") {
				continue
			}
			if isSSE && strings.EqualFold(k, "Content-Length") {
				continue
			}
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)

		// 流式 Flush 透传（支持 SSE 与实时流式生成）
		flusher, isFlusher := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			select {
			case <-r.Context().Done():
				// 客户端主动中断（如停止生成、取消请求），快速 drain 剩余 buffer 并退出，保障连接池复用
				io.CopyN(io.Discard, resp.Body, 64*1024)
				return
			default:
			}

			n, rErr := resp.Body.Read(buf)
			if n > 0 {
				if _, wErr := w.Write(buf[:n]); wErr != nil {
					// 客户端网络断开
					io.CopyN(io.Discard, resp.Body, 64*1024)
					return
				}
				if isFlusher {
					flusher.Flush()
				}
			}
			if rErr != nil {
				break
			}
		}
		return
	}

	// 全部重试用尽
	if lastStatusCode != 0 {
		s.writeUpstreamError(w, lastStatusCode, lastErrBody)
	} else {
		http.Error(w, `{"error":{"message":"所有号池重试用尽","type":"retries_exhausted"}}`, http.StatusBadGateway)
	}
}

func (s *GatewayServer) writeUpstreamError(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if len(body) > 0 {
		w.Write(body)
	} else {
		w.Write([]byte(fmt.Sprintf(`{"error":{"message":"上游返回状态码 %d","code":"upstream_%d"}}`, status, status)))
	}
}

func main() {
	port := flag.Int("port", 8080, "网关监听端口")
	upstream := flag.String("upstream", "https://agnes-ai.cn", "Agnes 上游根地址")
	cooldown := flag.Int("cooldown", 120, "429 冷却时长(秒)")
	retries := flag.Int("retries", 3, "故障自动重试次数")
	keysFile := flag.String("keys-file", "", "包含 Key 列表的文件(一行一个)")
	dnsFlag := flag.String("dns", "", "逗号分隔的 DNS 服务器(如 223.5.5.5,119.29.29.29)")
	keysFlag := flag.String("keys", "", "逗号分隔的 Key 列表")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("==================================================")
	log.Printf("   Agnes 手机反代网关核心 (Agnes Gateway Core)     ")
	log.Printf("==================================================")
	log.Printf("上游地址: %s", *upstream)
	log.Printf("监听端口: %d", *port)
	log.Printf("冷却退避: %d 秒", *cooldown)

	// 装配 DNS：优先使用 App 传入的系统 DNS，其次国内公共 DNS 兜底
	var dnsServers []string
	seenDNS := make(map[string]bool)
	appendDNS := func(raw string) {
		if s, ok := normalizeDNSServer(raw); ok && !seenDNS[s] {
			seenDNS[s] = true
			dnsServers = append(dnsServers, s)
		}
	}
	if *dnsFlag != "" {
		for _, raw := range strings.Split(*dnsFlag, ",") {
			appendDNS(raw)
		}
	}
	for _, raw := range []string{"223.5.5.5", "119.29.29.29", "114.114.114.114", "8.8.8.8"} {
		appendDNS(raw)
	}

	upstreamHost := ""
	if u, err := url.Parse(*upstream); err == nil {
		upstreamHost = u.Hostname()
	}
	usableDNS := probeDNSServers(dnsServers, upstreamHost)
	if len(usableDNS) == 0 {
		log.Printf("[DNS] 候选 DNS 均探测失败，按默认顺序尝试: %s", strings.Join(dnsServers, ", "))
		usableDNS = dnsServers
	} else {
		log.Printf("[DNS] 可用 DNS 服务器: %s", strings.Join(usableDNS, ", "))
	}
	customResolver = newResolver(usableDNS)

	pool := NewPoolManager(*upstream, *cooldown)

	// 从参数或文件加载初始 Key
	var initialKeys []string
	if *keysFlag != "" {
		initialKeys = append(initialKeys, strings.Split(*keysFlag, ",")...)
	}
	if *keysFile != "" {
		if content, err := os.ReadFile(*keysFile); err == nil {
			initialKeys = append(initialKeys, strings.Split(string(content), "\n")...)
		}
	}
	// 从环境变量加载初始 Key
	if envKeys := os.Getenv("AGNES_KEYS"); envKeys != "" {
		initialKeys = append(initialKeys, strings.Split(envKeys, ",")...)
	}

	pool.SetKeys(initialKeys)

	server := NewGatewayServer(pool, *port, *retries)
	httpServer := &http.Server{
		Addr:    server.listenAddr,
		Handler: server,
	}

	go func() {
		log.Printf("网关服务已启动，局域网监听地址: http://0.0.0.0:%d/v1", *port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("网关服务异常退出: %v", err)
		}
	}()

	// 捕获优雅退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("收到退出信号，正在优雅关闭网关...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	log.Printf("网关服务已安全终止。")
}
