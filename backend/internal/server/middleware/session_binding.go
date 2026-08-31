package middleware

import (
	"net"
	"net/netip"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SessionBindingContext 全局中间件：将请求的客户端 IP 与 User-Agent 注入
// request context，供 token 签发路径（登录 / 刷新 / OAuth 回调）读取并写入会话绑定，
// 同时作为审计日志、会话绑定校验的统一客户端 IP 来源。
// IP 取值与 API Key IP 限制共用转发 IP 开关：开启时旧版原始转发头逻辑
// 接管解析，关闭时使用 Gin 的 server.trusted_proxies 可信代理链。
func SessionBindingContext(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		forwardedIPSettings := cfg.ForwardedClientIPSettings()
		ip.SetForwardedIPSettings(c, forwardedIPSettings.TrustForwardedIP, forwardedIPSettings.Headers)
		userAgent := normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes)
		c.Request.Header.Set("User-Agent", userAgent)
		binding := &service.SessionBinding{
			IP:        ip.GetSecurityClientIP(c, forwardedIPSettings.TrustForwardedIP),
			UserAgent: userAgent,
		}
		fingerprints := append([]string{}, c.Request.Header.Values("X-Browser-Fingerprint")...)
		fingerprints = append(fingerprints, c.Request.Header.Values("X-Browser-Fingerprints")...)
		ja3, ja4 := "", ""
		if trustedProxyFingerprintHeaders(c, cfg) {
			ja3 = firstHeader(c, "X-JA3", "X-JA3-Fingerprint", "X-JA3-Hash", "X-TLS-JA3", "X-Client-JA3", "CF-JA3-Fingerprint", "CF-JA3-Hash", "JA3")
			ja4 = firstHeader(c, "X-JA4", "X-JA4-Fingerprint", "X-JA4-Hash", "X-TLS-JA4", "X-Client-JA4", "CF-JA4-Fingerprint", "CF-JA4-Hash", "CF-JA4", "JA4")
		}
		requestContext := service.WithSessionBinding(c.Request.Context(), binding)
		requestContext = service.WithRiskSignals(requestContext, service.RiskSignals{IPAddress: binding.IP, UserAgent: userAgent, BrowserFingerprints: fingerprints, JA3: ja3, JA4: ja4})
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func trustedProxyFingerprintHeaders(c *gin.Context, cfg *config.Config) bool {
	if c == nil || cfg == nil || !cfg.Server.TrustedProxiesConfigured || len(cfg.Server.TrustedProxies) == 0 {
		return false
	}
	remote := strings.TrimSpace(c.Request.RemoteAddr)
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	remote = strings.Trim(strings.TrimSpace(remote), "[]")
	addr, err := netip.ParseAddr(remote)
	if err != nil {
		return false
	}
	for _, raw := range cfg.Server.TrustedProxies {
		raw = strings.TrimSpace(raw)
		if prefix, parseErr := netip.ParsePrefix(raw); parseErr == nil && prefix.Contains(addr) {
			return true
		}
		if proxyAddr, parseErr := netip.ParseAddr(raw); parseErr == nil && proxyAddr == addr {
			return true
		}
	}
	return false
}

func firstHeader(c *gin.Context, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
			return value
		}
	}
	return ""
}

// requestSessionBinding 返回当前请求的会话指纹，优先取 SessionBindingContext
// 注入的解析结果（保证与 token 签发路径取值一致）；注入缺失时使用安全回退。
func requestSessionBinding(c *gin.Context) *service.SessionBinding {
	if binding := service.SessionBindingFromContext(c.Request.Context()); binding != nil {
		return binding
	}
	return &service.SessionBinding{
		IP:        ip.GetTrustedClientIP(c),
		UserAgent: normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes),
	}
}

// SecurityClientIP 返回当前请求用于安全敏感记录（审计日志等）的客户端 IP。
// 与会话绑定、API Key IP 限制共用同一套客户端 IP 来源。
func SecurityClientIP(c *gin.Context) string {
	if binding := service.SessionBindingFromContext(c.Request.Context()); binding != nil &&
		strings.TrimSpace(binding.IP) != "" {
		return binding.IP
	}
	return ip.GetTrustedClientIP(c)
}

// enforceSessionBinding 校验 access token 的会话指纹（IP/UA 绑定）。
// 指纹不匹配时：撤销该会话家族的所有 refresh token、写入审计安全事件、返回 401。
// 返回 false 表示请求已被中断。
//
// 兼容性：claims.BindingHash 为空（功能上线前签发的旧 token）时放行，
// 该会话在下一次 refresh 轮转时会自动获得绑定。
func enforceSessionBinding(
	c *gin.Context,
	authService *service.AuthService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
	claims *service.JWTClaims,
) bool {
	if settingService == nil || !settingService.IsSessionBindingEnabled(c.Request.Context()) {
		return true
	}
	if claims == nil || claims.BindingHash == "" {
		return true
	}
	binding := requestSessionBinding(c)
	current := binding.Hash()
	if current == "" || current == claims.BindingHash {
		return true
	}

	if authService != nil {
		_ = authService.RevokeSessionFamily(c.Request.Context(), claims.SessionID)
	}
	if auditService != nil {
		uid := claims.UserID
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		auditService.Record(&service.AuditLog{
			ActorUserID: &uid,
			ActorEmail:  claims.Email,
			ActorRole:   claims.Role,
			AuthMethod:  service.AuditAuthMethodJWT,
			Action:      service.AuditActionSessionBindingMismatch,
			Method:      c.Request.Method,
			Path:        path,
			ClientIP:    binding.IP,
			UserAgent:   normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes),
			StatusCode:  401,
		})
	}
	AbortWithError(c, 401, "SESSION_BINDING_MISMATCH", "Session network fingerprint changed, please login again")
	return false
}
