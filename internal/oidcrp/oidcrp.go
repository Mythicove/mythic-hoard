// Package oidcrp 实现最小 OpenID Connect 依赖方（授权码流）。
// 校验全部委托 go-oidc：discovery、JWKS 轮换、签名/iss/aud/exp、nonce。
// 机密客户端（client_secret），不启用 PKCE。
package oidcrp

import (
	"context"
	"errors"
	"fmt"

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
}

// New 执行 discovery 并构造 RP（issuer 不可达时返回错误）。
func New(ctx context.Context, cfg Config) (*RP, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return nil, errors.New("oidcrp: issuer and client_id are required")
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
	}, nil
}

// AuthCodeURL 生成授权跳转地址（调用方负责生成并保管 state/nonce）。
func (rp *RP) AuthCodeURL(state, nonce string) string {
	return rp.oauth2.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce))
}

// Exchange 用授权码换 token 并校验 ID token（签名/iss/aud/exp/nonce），返回用户声明。
func (rp *RP) Exchange(ctx context.Context, code, nonce string) (*Claims, error) {
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
