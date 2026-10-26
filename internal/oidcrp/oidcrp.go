// Package oidcrp 实现最小 OpenID Connect 依赖方（授权码流）。
// 校验全部委托 go-oidc：discovery、JWKS 轮换、签名/iss/aud/exp、nonce。
// 机密客户端（client_secret），不启用 PKCE。
package oidcrp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config 是 RP 配置。
type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string // 缺省 openid profile email
	// InternalBaseURL 是容器/内网侧可达的 IdP 基址（split-horizon）：
	// issuer 是浏览器面地址（如 http://localhost:8080），服务端调用
	// （discovery/JWKS/token）改走 InternalBaseURL（如 http://host.docker.internal:8080），
	// Host 头保留 issuer 以匹配 IdP 的 vhost 与 iss 校验。留空则直连 issuer。
	InternalBaseURL string
}

// Claims 是 ID token 中本项目消费的用户声明（Mirror IdP 提供 sub/preferred_username/email/nickname/player）。
type Claims struct {
	Sub               string `json:"sub"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Nickname          string `json:"nickname"`
	Player            string `json:"player"`
}

// DisplayName 返回最佳展示名。
func (c Claims) DisplayName() string {
	for _, v := range []string{c.PreferredUsername, c.Nickname, c.Player, c.Email} {
		if v != "" {
			return v
		}
	}
	return c.Sub
}

// RP 是 OIDC 依赖方。
type RP struct {
	oauth2   oauth2.Config
	verifier *oidc.IDTokenVerifier
	http     *http.Client
}

// rewriteTransport 把发往 issuer 的服务端请求改道到内网基址。
type rewriteTransport struct {
	base http.RoundTripper
	from string
	to   *url.URL
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == t.from {
		r2 := req.Clone(req.Context())
		r2.URL.Scheme = t.to.Scheme
		r2.URL.Host = t.to.Host
		r2.Host = t.from
		req = r2
	}
	return t.base.RoundTrip(req)
}

func internalClient(issuer, internalBase string) *http.Client {
	if internalBase == "" {
		return nil
	}
	from, err1 := url.Parse(issuer)
	to, err2 := url.Parse(internalBase)
	if err1 != nil || err2 != nil || from.Host == "" || to.Host == "" {
		return nil
	}
	return &http.Client{
		Transport: rewriteTransport{base: http.DefaultTransport, from: from.Host, to: to},
	}
}

// New 执行 discovery 并构造 RP（issuer 不可达时返回错误）。
func New(ctx context.Context, cfg Config) (*RP, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return nil, errors.New("oidcrp: issuer and client_id are required")
	}
	client := internalClient(cfg.Issuer, cfg.InternalBaseURL)
	if client != nil {
		ctx = oidc.ClientContext(ctx, client)
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}
	return &RP{
		oauth2: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       scopes,
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		http:     client,
	}, nil
}

// AuthCodeURL 生成授权跳转地址（调用方负责生成并保管 state/nonce）。
func (rp *RP) AuthCodeURL(state, nonce string) string {
	return rp.oauth2.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce))
}

// Exchange 用授权码换 token 并校验 ID token（签名/iss/aud/exp/nonce），返回用户声明。
func (rp *RP) Exchange(ctx context.Context, code, nonce string) (*Claims, error) {
	if rp.http != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, rp.http)
	}
	tok, err := rp.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		return nil, errors.New("id_token missing in token response")
	}
	idToken, err := rp.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}
	if idToken.Nonce != nonce {
		return nil, errors.New("nonce mismatch")
	}
	var c Claims
	if err := idToken.Claims(&c); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}
	if c.Sub == "" {
		return nil, errors.New("empty sub claim")
	}
	return &c, nil
}
