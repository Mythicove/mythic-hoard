package hoard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// PATPrefix 是个人访问令牌的明文前缀（用于区分 admin token 与会话）。
const PATPrefix = "mhp_"

// scopesKey 是 PAT scopes 的 context 键。
type scopesKey struct{}

// ScopesFromContext 返回 PAT 授权的 scopes（会话登录为 nil）。
func ScopesFromContext(ctx context.Context) []string {
	sc, _ := ctx.Value(scopesKey{}).([]string)
	return sc
}

// generatePAT 生成明文令牌与其 sha256 哈希。
func generatePAT() (token, hash string) {
	token = PATPrefix + randomHex(20)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:])
}

// hashPAT 计算明文令牌的存储哈希。
func hashPAT(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// handleCreateTeam 创建团队（调用者成为 owner）。
func (s *Service) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id, err := s.store.CreateTeam(r.Context(), strings.TrimSpace(req.Name), u.Sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// handleListMyTeams 列出我所在的团队。
func (s *Service) handleListMyTeams(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	teams, err := s.store.ListUserTeams(r.Context(), u.Sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, teams)
}

// handleListTeamMembers 列出团队成员（公开，与 Modrinth 一致）。
func (s *Service) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	members, err := s.store.ListTeamMembers(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if err == ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// canManageTeam 返回用户是否可管理团队成员（团队 owner 或站点 admin）。
func (s *Service) canManageTeam(r *http.Request, teamID string) bool {
	u := UserFromContext(r.Context())
	if u.IsAdmin() {
		return true
	}
	m, err := s.store.GetTeamMember(r.Context(), teamID, u.Sub)
	return err == nil && m.Role == "owner"
}

// handleAddTeamMember 添加/变更成员（owner/admin）。
// isLastOwner 报告 sub 是否为团队唯一 owner。末位 owner 离开或被降级会让团队失去
// 管理者（无人能再加人、改状态、删项目），故这类操作必须先转移所有权。
func (s *Service) isLastOwner(ctx context.Context, teamID, sub string) (bool, error) {
	members, err := s.store.ListTeamMembers(ctx, teamID)
	if err != nil {
		return false, err
	}
	owners := 0
	targetIsOwner := false
	for _, m := range members {
		if m.Role != "owner" {
			continue
		}
		owners++
		if m.UserSub == sub {
			targetIsOwner = true
		}
	}
	return targetIsOwner && owners <= 1, nil
}

func (s *Service) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	if !s.canManageTeam(r, teamID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Sub  string `json:"sub"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Sub == "" {
		http.Error(w, "sub required", http.StatusBadRequest)
		return
	}
	if req.Role != "owner" && req.Role != "member" {
		req.Role = "member"
	}
	// 降级唯一 owner 等同把团队变成孤儿
	if req.Role != "owner" {
		if existing, err := s.store.GetTeamMember(r.Context(), teamID, req.Sub); err == nil && existing.Role == "owner" {
			last, err := s.isLastOwner(r.Context(), teamID, req.Sub)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if last {
				http.Error(w, "team needs at least one owner", http.StatusConflict)
				return
			}
		}
	}
	if err := s.store.AddTeamMember(r.Context(), teamID, req.Sub, req.Role); err != nil {
		if err == ErrNotFound {
			http.Error(w, "team or user not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveTeamMember 移除成员（owner/admin）或自己退出。
func (s *Service) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	sub := chi.URLParam(r, "sub")
	u := UserFromContext(r.Context())
	if sub != u.Sub && !s.canManageTeam(r, teamID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	last, err := s.isLastOwner(r.Context(), teamID, sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if last {
		http.Error(w, "last owner cannot leave; transfer ownership first", http.StatusConflict)
		return
	}
	if err := s.store.RemoveTeamMember(r.Context(), teamID, sub); err != nil {
		if err == ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListPATs 列出我的 PAT（元数据，无明文）。
func (s *Service) handleListPATs(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	pats, err := s.store.ListPATs(r.Context(), u.Sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, pats)
}

// handleCreatePAT 创建 PAT；明文 token 仅此一次返回。
func (s *Service) handleCreatePAT(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	var req struct {
		Name          string   `json:"name"`
		Scopes        []string `json:"scopes"`
		ExpiresInDays int      `json:"expires_in_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	var expires *time.Time
	if req.ExpiresInDays > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
		expires = &t
	}
	token, hash := generatePAT()
	id, err := s.store.CreatePAT(r.Context(), u.Sub, strings.TrimSpace(req.Name), hash, req.Scopes, expires)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "token": token})
}

// handleDeletePAT 撤销 PAT。
func (s *Service) handleDeletePAT(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if err := s.store.DeletePAT(r.Context(), u.Sub, chi.URLParam(r, "id")); err != nil {
		if err == ErrNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
