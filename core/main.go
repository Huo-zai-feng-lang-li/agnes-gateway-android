package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// localLocation 确保日志与显示时间与用户本地时区严格一致
var localLocation *time.Location

func init() {
	log.SetFlags(0)
	// 优先检测系统时区，若为 UTC/无效则尝试根据 TZ 环境变量或默认 CST (UTC+8) 校准
	loc := time.Local
	if tz := os.Getenv("TZ"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	if loc == nil || loc == time.UTC {
		localLocation = time.FixedZone("CST", 8*3600)
	} else {
		localLocation = loc
	}
}

// formatLogTime 返回规范的当前时区时间格式 HH:mm:ss
func formatLogTime() string {
	return time.Now().In(localLocation).Format("15:04:05")
}

// KeyState 记录单个 Key 的运行时健康状态
type KeyState struct {
	// Key 为上游明文凭证，禁止出现在任何 JSON 输出中（管理接口虽限本机回环，仍不应出网）
	Key          string    `json:"-"`
	Alias        string    `json:"alias"`
	MaskedKey    string    `json:"masked_key"`
	DisplayName  string    `json:"display_name"`
	CooldownTill time.Time `json:"cooldown_till"`
	FailureCount int64     `json:"failure_count"`
	SuccessCount int64     `json:"success_count"`
	LastError    string    `json:"last_error"`
	Disabled     bool      `json:"disabled"`
}

// parseKeyEntry 解析包含可选备注名的 Key 行
// 支持格式：
// 1) sk-xxxx#备注名
// 2) sk-xxxx----备注名
// 3) sk-xxxx:备注名
// 4) sk-xxxx 备注名
// 5) 裸 sk-xxxx
func parseKeyEntry(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	delimiters := []string{"----", "#", " ", "\t"}
	for _, delim := range delimiters {
		if idx := strings.Index(raw, delim); idx > 0 {
			key := strings.TrimSpace(raw[:idx])
			alias := strings.TrimSpace(raw[idx+len(delim):])
			if key != "" {
				return key, alias
			}
		}
	}
	// 尝试单个英文冒号分隔（避免与 url 冲突，key 通常不含 :）
	if idx := strings.Index(raw, ":"); idx > 0 && !strings.Contains(raw[:idx], "/") {
		key := strings.TrimSpace(raw[:idx])
		alias := strings.TrimSpace(raw[idx+1:])
		if key != "" {
			return key, alias
		}
	}

	return raw, ""
}

// formatKeyDisplayName 格式化 Key 展示名称：截取末尾字符，有备注时展示备注
func formatKeyDisplayName(key, alias string) (string, string) {
	key = strings.TrimSpace(key)
	var masked string
	if len(key) <= 8 {
		masked = "******"
	} else {
		// 截取末尾 4 个字符，如 ...piLR
		masked = "..." + key[len(key)-4:]
	}

	if alias != "" {
		return fmt.Sprintf("%s(%s)", alias, masked), masked
	}
	return masked, masked
}

// PoolManager 负责多 Key 轮询与智能退避调度
type PoolManager struct {
	mu           sync.RWMutex
	keys         []*KeyState
	bannedCount  int64
	bannedKeys   []string // 因 401 被物理剔除的原始 Key 黑名单（供客户端定向清理）
	currentIndex uint64
	upstreamURL  string
	cooldownSec  int
	lastUsedKey  atomic.Pointer[string]
}

func extractModelName(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	return probe.Model
}

func NewPoolManager(upstream string, cooldownSec int) *PoolManager {
	if upstream == "" {
		upstream = "https://apihub.agnes-ai.cn/v1"
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

func (p *PoolManager) SetKeys(rawEntries []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	keyMap := make(map[string]*KeyState)
	for _, old := range p.keys {
		keyMap[old.Key] = old
	}

	var newKeys []*KeyState
	seen := make(map[string]bool)
	for _, entry := range rawEntries {
		k, alias := parseKeyEntry(entry)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		disp, masked := formatKeyDisplayName(k, alias)

		if old, exists := keyMap[k]; exists {
			old.Alias = alias
			old.DisplayName = disp
			old.MaskedKey = masked
			newKeys = append(newKeys, old)
		} else {
			newKeys = append(newKeys, &KeyState{
				Key:         k,
				Alias:       alias,
				MaskedKey:   masked,
				DisplayName: disp,
			})
		}
	}
	p.keys = newKeys
	// 用户重新加入的 Key 视为已恢复，移出封禁黑名单，避免被客户端再次误清
	kept := make([]string, 0, len(p.bannedKeys))
	removedFromBanned := 0
	for _, bk := range p.bannedKeys {
		if !seen[bk] {
			kept = append(kept, bk)
		} else {
			removedFromBanned++
		}
	}
	p.bannedKeys = kept
	if removedFromBanned > 0 {
		atomic.AddInt64(&p.bannedCount, -int64(removedFromBanned))
	}
	log.Printf("%s [系统] 号池已加载 %d 个有效 Key", formatLogTime(), len(p.keys))
}

// PickNext 轮询挑选一个当前未处于冷却中的 Key
func (p *PoolManager) PickNext() (*KeyState, int) {
	p.mu.Lock()
	defer p.mu.Unlock()

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
		disp := ks.DisplayName
		p.lastUsedKey.Store(&disp)
		return ks, total
	}

	// 所有 Key 均处于冷却/禁用状态：直接返回 nil，由上层快速 503，
	// 避免再挑一个冷却中的 Key 白白发起一次注定失败的上游请求（几百 ms ~ 3s）。
	// total 保持原值，供调用方区分「空池」(total=0) 与「全冷却」(total>0)。
	return nil, total
}

func (p *PoolManager) MarkSuccess(ks *KeyState) {
	atomic.AddInt64(&ks.SuccessCount, 1)
}

// allUnavailableLocked 判断号池是否已无可用 Key（空池 / 全部冷却 / 全部禁用）。调用方需持有 p.mu。
func (p *PoolManager) allUnavailableLocked(now time.Time) bool {
	for _, ks := range p.keys {
		if !ks.Disabled && !now.Before(ks.CooldownTill) {
			return false
		}
	}
	return true
}

// logAllUnavailable 输出统一告警日志，供 Android 前台服务在后台直接识别并触发强提醒
func logAllUnavailable() {
	log.Printf("%s [告警] 号池全部不可用 (所有Key均被限流或已失效)", formatLogTime())
}

func (p *PoolManager) MarkCooldown(ks *KeyState, errReason string, duration time.Duration) {
	if duration <= 0 {
		duration = time.Duration(p.cooldownSec) * time.Second
	}
	atomic.AddInt64(&ks.FailureCount, 1)
	p.mu.Lock()
	ks.CooldownTill = time.Now().Add(duration)
	ks.LastError = errReason
	dead := p.allUnavailableLocked(time.Now())
	disp := ks.DisplayName
	p.mu.Unlock()
	log.Printf("%s [限流] 429 速率限制 | Key: %s -> 冷却 %v", formatLogTime(), disp, duration)
	if dead {
		logAllUnavailable()
	}
}

// displayNameOf 在锁保护下读取 Key 展示名，避免与 SetKeys 并发写入构成数据竞争
func (p *PoolManager) displayNameOf(ks *KeyState) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return ks.DisplayName
}

// RemoveKey 检测到 401 封禁时从号池中物理剔除该 Key，并记入封禁黑名单
func (p *PoolManager) RemoveKey(key string, errReason string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	removed := false
	var updated []*KeyState
	var targetDisp string
	for _, ks := range p.keys {
		if ks.Key == key {
			removed = true
			targetDisp = ks.DisplayName
			atomic.AddInt64(&p.bannedCount, 1)
			continue
		}
		updated = append(updated, ks)
	}
	if removed {
		p.keys = updated
		p.bannedKeys = append(p.bannedKeys, key)
		log.Printf("%s [封禁] 401 凭证失效 | Key: %s -> 已从号池剔除", formatLogTime(), targetDisp)
		if p.allUnavailableLocked(time.Now()) {
			logAllUnavailable()
		}
	}
	return removed
}

// BannedKeys 返回因 401 被剔除的原始 Key 黑名单副本
func (p *PoolManager) BannedKeys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append(make([]string, 0, len(p.bannedKeys)), p.bannedKeys...)
}

func (p *PoolManager) Snapshot() ([]KeyState, string, string, int, int, int) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	now := time.Now()
	list := make([]KeyState, len(p.keys))
	available := 0
	cooldown := 0

	for i, ks := range p.keys {
		list[i] = *ks
		if !ks.Disabled {
			if !now.Before(ks.CooldownTill) {
				available++
			} else {
				cooldown++
			}
		}
	}
	banned := int(atomic.LoadInt64(&p.bannedCount))
	lastUsed := ""
	if ptr := p.lastUsedKey.Load(); ptr != nil {
		lastUsed = *ptr
	}
	return list, p.upstreamURL, lastUsed, available, cooldown, banned
}

// ==========================================
// 极速 DNS 竞速与内存缓存解析器 (<1s 性能保证)
// ==========================================

type dnsCacheEntry struct {
	ips     []net.IP
	expires time.Time
}

type FastDNSResolver struct {
	servers []string
	cache   sync.Map
	ttl     time.Duration
}

func NewFastDNSResolver(servers []string) *FastDNSResolver {
	return &FastDNSResolver{
		servers: servers,
		ttl:     10 * time.Minute, // 10分钟长效缓存，彻底消除每次请求的 DNS 延迟
	}
}

// ResolveHost 并发竞速向可用 DNS 查询 host，并进行内存缓存
func (r *FastDNSResolver) ResolveHost(ctx context.Context, host string) ([]net.IP, error) {
	if host == "" {
		return nil, fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}

	// 1. 优先查缓存 (0ms 极速命中)
	if val, ok := r.cache.Load(host); ok {
		entry := val.(dnsCacheEntry)
		if time.Now().Before(entry.expires) && len(entry.ips) > 0 {
			return entry.ips, nil
		}
	}

	// 2. 缓存未命中或过期，并发竞速查询 (Fastest Race)
	if len(r.servers) == 0 {
		// 回退系统解析
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err == nil && len(ips) > 0 {
			r.cache.Store(host, dnsCacheEntry{ips: ips, expires: time.Now().Add(r.ttl)})
		}
		return ips, err
	}

	type raceRes struct {
		ips []net.IP
		err error
	}
	ch := make(chan raceRes, len(r.servers))
	raceCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()

	for _, s := range r.servers {
		go func(server string) {
			subResolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
					d := &net.Dialer{Timeout: 800 * time.Millisecond}
					return d.DialContext(ctx, network, server)
				},
			}
			ips, err := subResolver.LookupIP(raceCtx, "ip", host)
			ch <- raceRes{ips: ips, err: err}
		}(s)
	}

	var firstErr error
	for i := 0; i < len(r.servers); i++ {
		select {
		case res := <-ch:
			if res.err == nil && len(res.ips) > 0 {
				// 最快成功的立即写入缓存，并取消其余 DNS 查询协程
				cancel()
				r.cache.Store(host, dnsCacheEntry{ips: res.ips, expires: time.Now().Add(r.ttl)})
				return res.ips, nil
			}
			if firstErr == nil && res.err != nil {
				firstErr = res.err
			}
		case <-raceCtx.Done():
			return nil, raceCtx.Err()
		}
	}

	if firstErr == nil {
		firstErr = fmt.Errorf("all dns servers failed to resolve %s", host)
	}
	return nil, firstErr
}

var globalFastResolver *FastDNSResolver

// maxDialIPs 单次拨号最多尝试的候选 IP 数，防止最坏情况下累计超时过长
const maxDialIPs = 4

// orderIPsV4First 将 IPv4 地址排在前面（稳定顺序），避免无 IPv6 路由网络下首选 AAAA 挂起
func orderIPsV4First(ips []net.IP) []net.IP {
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip.To4() != nil {
			out = append(out, ip)
		}
	}
	for _, ip := range ips {
		if ip.To4() == nil {
			out = append(out, ip)
		}
	}
	return out
}

// dialIPsFallback 并发竞速拨号（Happy Eyeballs 变体）：IPv4 优先，首个立即拨号，
// 后续每 50ms 错峰启动，谁先连通就用谁，其余立即取消。
// 消除「首个 IP 是黑洞时串行等待满超时」导致的额外首字延迟。
func dialIPsFallback(ctx context.Context, d *net.Dialer, network string, ips []net.IP, port string) (net.Conn, error) {
	ordered := orderIPsV4First(ips)
	if len(ordered) > maxDialIPs {
		ordered = ordered[:maxDialIPs]
	}
	if len(ordered) == 0 {
		return nil, fmt.Errorf("no candidate ip to dial")
	}

	type dialResult struct {
		conn net.Conn
		err  error
	}
	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan dialResult, len(ordered))
	for i, ip := range ordered {
		go func(idx int, ip net.IP) {
			// 错峰启动：首个立即拨号，后续每档 +50ms，避免同时打爆对端并保证 v4 优先
			if idx > 0 {
				select {
				case <-time.After(time.Duration(idx) * 50 * time.Millisecond):
				case <-raceCtx.Done():
					ch <- dialResult{err: raceCtx.Err()}
					return
				}
			}
			conn, err := d.DialContext(raceCtx, network, net.JoinHostPort(ip.String(), port))
			if err != nil {
				ch <- dialResult{err: err}
				return
			}
			ch <- dialResult{conn: conn}
		}(i, ip)
	}

	var lastErr error
	for i := 0; i < len(ordered); i++ {
		select {
		case res := <-ch:
			if res.err == nil {
				cancel()
				return res.conn, nil
			}
			lastErr = res.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("dial failed")
	}
	return nil, lastErr
}

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

	// 极速上游长连接传输层（预热复用 + Keep-Alive + 内存缓存 DNS）
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
				port = "443"
			}
			// 使用极速并发缓存 DNS 解析；IPv4 优先并逐 IP 回退，全部失败再走系统直连兜底
			if globalFastResolver != nil {
				ips, rErr := globalFastResolver.ResolveHost(ctx, host)
				if rErr == nil && len(ips) > 0 {
					// 并发拨号下单 IP 无需长超时：800ms 内不响应的 IP 直接放弃，由其他 IP 竞速胜出
					d := &net.Dialer{
						Timeout:   800 * time.Millisecond,
						KeepAlive: 30 * time.Second,
					}
					if conn, dErr := dialIPsFallback(ctx, d, network, ips, port); dErr == nil {
						return conn, nil
					}
				}
			}
			// 兜底系统直连
			d := &net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return d.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   30,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &GatewayServer{
		pool: pool,
		client: &http.Client{
			Transport: transport,
			Timeout:   0, // 流式长响应不设全局超时
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

	// 管理接口 (仅限本机回环访问)
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
			"time":     formatLogTime(),
		})
		return
	}

	// 核心业务反代转发 (OpenAI / v1 / responses 协议)
	s.forwardRequest(w, r)
}

func (s *GatewayServer) handleInternalAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/_gateway/status":
		list, upstream, lastUsed, available, cooldown, banned := s.pool.Snapshot()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]any{
			"upstream":        upstream,
			"last_used_key":   lastUsed,
			"total_keys":      len(list),
			"available_keys":  available,
			"cooldown_keys":   cooldown,
			"banned_keys":     banned,
			"banned_key_list": s.pool.BannedKeys(),
			"keys":            list,
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

	reqModel := extractModelName(reqBody)
	if reqModel == "" && strings.Contains(r.URL.Path, "models") {
		reqModel = "models"
	}

	var lastStatusCode int
	var lastErrBody []byte

	for attempt := 0; attempt < s.maxRetries; attempt++ {
		keyState, totalKeys := s.pool.PickNext()
		if keyState == nil {
			logAllUnavailable()
			if totalKeys > 0 {
				// 全冷却：立即 503，让调用方马上知道该等，而不是干等一次注定 429 的上游往返
				w.Header().Set("Retry-After", "30")
				http.Error(w, fmt.Sprintf(`{"error":{"message":"Agnes 号池当前全部处于冷却中（共 %d 个 Key），请稍后重试","type":"gateway_pool_cooldown"}}`, totalKeys), http.StatusServiceUnavailable)
			} else {
				http.Error(w, `{"error":{"message":"Agnes 号池为空，请先配置 API Key","type":"gateway_pool_empty"}}`, http.StatusServiceUnavailable)
			}
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

		for k, v := range r.Header {
			if strings.EqualFold(k, "Connection") || strings.EqualFold(k, "Keep-Alive") || strings.EqualFold(k, "Host") {
				continue
			}
			outReq.Header[k] = v
		}
		outReq.Header.Set("Authorization", "Bearer "+keyState.Key)

		// 记录发起单次上游调用的精准耗时（首字时间 TTFB）
		subStart := time.Now()
		resp, reqErr := s.client.Do(outReq)
		ttfbSec := time.Since(subStart).Seconds()

		if reqErr != nil {
			// 客户端主动断开/超时导致 ctx 取消，不是 Key 的问题，不能冷却号池
			if errors.Is(reqErr, context.Canceled) || errors.Is(reqErr, context.DeadlineExceeded) {
				log.Printf("%s [中断] 客户端断开 (%.2fs) | Key: %s", formatLogTime(), ttfbSec, s.pool.displayNameOf(keyState))
				return
			}
			log.Printf("%s [失败] 连接异常 (%.2fs) | Key: %s | %v", formatLogTime(), ttfbSec, s.pool.displayNameOf(keyState), reqErr)
			s.pool.MarkCooldown(keyState, reqErr.Error(), 30*time.Second)
			if totalKeys > 1 {
				continue
			}
			http.Error(w, fmt.Sprintf(`{"error":{"message":"连接 Agnes 上游失败: %v","type":"upstream_connect_error"}}`, reqErr), http.StatusBadGateway)
			return
		}

		// 针对 429 速率限制退避重试 (30 秒冷却)
		if resp.StatusCode == http.StatusTooManyRequests {
			errBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastStatusCode = resp.StatusCode
			lastErrBody = errBytes
			s.pool.MarkCooldown(keyState, "429_rate_limit", 30*time.Second)
			if totalKeys > 1 {
				continue
			}
			s.writeUpstreamError(w, resp.StatusCode, errBytes)
			return
		}

		// 针对 401 凭证无效（失效）直接从号池彻底移除
		if resp.StatusCode == http.StatusUnauthorized {
			errBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastStatusCode = resp.StatusCode
			lastErrBody = errBytes
			s.pool.RemoveKey(keyState.Key, "401_unauthorized")
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
			log.Printf("%s [失败] HTTP %d (%.2fs) | Key: %s", formatLogTime(), resp.StatusCode, ttfbSec, s.pool.displayNameOf(keyState))
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

		// 规范化输出极简日志：日志时间、成功状态、首字响应时间、模型、Key/备注
		modelPart := reqModel
		if modelPart == "" {
			modelPart = "default"
		}
		log.Printf("%s [成功] %.2fs | %s | Key: %s", formatLogTime(), ttfbSec, modelPart, s.pool.displayNameOf(keyState))

		flusher, isFlusher := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			select {
			case <-r.Context().Done():
				io.CopyN(io.Discard, resp.Body, 64*1024)
				return
			default:
			}

			n, rErr := resp.Body.Read(buf)
			if n > 0 {
				if _, wErr := w.Write(buf[:n]); wErr != nil {
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
	upstream := flag.String("upstream", "https://apihub.agnes-ai.cn/v1", "Agnes 上游根地址")
	cooldown := flag.Int("cooldown", 120, "429 冷却时长(秒)")
	retries := flag.Int("retries", 3, "故障自动重试次数")
	keysFile := flag.String("keys-file", "", "包含 Key 列表的文件(一行一个)")
	dnsFlag := flag.String("dns", "", "逗号分隔的 DNS 服务器(如 223.5.5.5,119.29.29.29)")
	keysFlag := flag.String("keys", "", "逗号分隔的 Key 列表")
	flag.Parse()

	// 不使用标准库默认微秒前缀，由业务代码统一提供 formatLogTime()
	log.SetFlags(0)

	log.Printf("==================================================")
	log.Printf("   Agnes 手机反代网关核心 (Agnes Gateway Core)     ")
	log.Printf("==================================================")
	log.Printf("%s [系统] 上游地址: %s", formatLogTime(), *upstream)
	log.Printf("%s [系统] 监听端口: %d", formatLogTime(), *port)
	log.Printf("%s [系统] 冷却退避: %d 秒", formatLogTime(), *cooldown)

	// 装配 DNS
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

	globalFastResolver = NewFastDNSResolver(dnsServers)
	log.Printf("%s [系统] 极速 DNS 并发缓存解析器已就绪 (候选: %s)", formatLogTime(), strings.Join(dnsServers, ", "))

	pool := NewPoolManager(*upstream, *cooldown)

	var initialKeys []string
	if *keysFlag != "" {
		initialKeys = append(initialKeys, strings.Split(*keysFlag, ",")...)
	}
	if *keysFile != "" {
		if content, err := os.ReadFile(*keysFile); err == nil {
			initialKeys = append(initialKeys, strings.Split(string(content), "\n")...)
		}
	}
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
		log.Printf("%s [系统] 网关服务已启动，局域网监听地址: http://0.0.0.0:%d/v1", formatLogTime(), *port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("%s [系统] 网关服务异常退出: %v", formatLogTime(), err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Printf("%s [系统] 收到退出信号，正在优雅关闭网关...", formatLogTime())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	log.Printf("%s [系统] 网关服务已安全终止。", formatLogTime())
}
