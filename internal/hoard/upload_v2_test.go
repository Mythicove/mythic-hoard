package hoard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

func multipartUpload(t *testing.T, dataJSON string, filename string, content []byte) (string, *bytes.Buffer) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("data", dataJSON)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(content)
	_ = mw.Close()
	return mw.FormDataContentType(), body
}

func TestUserUploadOwnershipFlow(t *testing.T) {
	ctx := context.Background()
	_, store, srv := newAuthTestRouter(t)
	defer srv.Close()

	_ = store.UpsertUser(ctx, User{Sub: "up-owner", Username: "Owner", Role: "user"})
	_ = store.UpsertUser(ctx, User{Sub: "up-stranger", Username: "Stranger", Role: "user"})
	_ = store.CreateSession(ctx, "sess-owner", "up-owner", time.Now().Add(time.Hour))
	_ = store.CreateSession(ctx, "sess-stranger", "up-stranger", time.Now().Add(time.Hour))

	do := func(method, path, contentType, body, sess string) *http.Response {
		var rdr io.Reader = strings.NewReader(body)
		req, _ := http.NewRequest(method, srv.URL+path, rdr)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if sess != "" {
			req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sess})
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	doBearer := func(method, path, contentType, body, token string) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// 未登录 → 401
	ct, body := multipartUpload(t, `{"title":"Anon Mod"}`, "mod.jar", []byte("bytes"))
	res := do(http.MethodPost, "/v2/project", ct, body.String(), "")
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon upload: %d", res.StatusCode)
	}

	// owner 创建项目 → 201 + team 归属
	data := `{"title":"Owner Mod","description":"d","project_type":"mod","loaders":["fabric"],"game_versions":["1.20.1"],"version_number":"1.0.0","version_name":"v1.0.0"}`
	ct, body = multipartUpload(t, data, "mod.jar", []byte("jar-bytes-1"))
	res = do(http.MethodPost, "/v2/project", ct, body.String(), "sess-owner")
	var proj Project
	_ = json.NewDecoder(res.Body).Decode(&proj)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || proj.TeamID == "" {
		t.Fatalf("create project: %d %+v", res.StatusCode, proj)
	}
	members, err := store.ListTeamMembers(ctx, proj.TeamID)
	if err != nil || len(members) != 1 || members[0].UserSub != "up-owner" || members[0].Role != "owner" {
		t.Fatalf("team members: %+v err=%v", members, err)
	}
	// 公开读可见 team 字段
	res = do(http.MethodGet, "/v2/project/"+proj.ID, "", "", "")
	var pub Project
	_ = json.NewDecoder(res.Body).Decode(&pub)
	res.Body.Close()
	if pub.TeamID != proj.TeamID {
		t.Fatalf("public team field: %+v", pub)
	}

	// owner 发新版本 → 201
	vdata := `{"project_id":"` + proj.ID + `","version_number":"1.1.0","loaders":["fabric"],"game_versions":["1.20.1"]}`
	ct, body = multipartUpload(t, vdata, "mod-1.1.jar", []byte("jar-bytes-2"))
	res = do(http.MethodPost, "/v2/version", ct, body.String(), "sess-owner")
	var ver Version
	_ = json.NewDecoder(res.Body).Decode(&ver)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || ver.VersionNumber != "1.1.0" {
		t.Fatalf("add version: %d %+v", res.StatusCode, ver)
	}

	// 陌生人发版 → 403
	ct, body = multipartUpload(t, vdata, "evil.jar", []byte("x"))
	res = do(http.MethodPost, "/v2/version", ct, body.String(), "sess-stranger")
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger add version: %d", res.StatusCode)
	}
	// 陌生人删项目 → 403
	res = do(http.MethodDelete, "/v2/project/"+proj.ID, "", "", "sess-stranger")
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger delete: %d", res.StatusCode)
	}

	// PAT：伪造 → 401；无 scopes → 可上传；READ-only → 403
	res = doBearer(http.MethodGet, "/auth/me", "", "", "mhp_unknown")
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bogus pat: %d", res.StatusCode)
	}
	tokOK, hashOK := generatePAT()
	tokRO, hashRO := generatePAT()
	_, _ = store.CreatePAT(ctx, "up-owner", "ci-ok", hashOK, nil, nil)
	_, _ = store.CreatePAT(ctx, "up-owner", "ci-readonly", hashRO, []string{"READ"}, nil)

	pd := `{"title":"PAT Mod","project_type":"mod","version_number":"0.1.0"}`
	ct, body = multipartUpload(t, pd, "pat.jar", []byte("pat-bytes"))
	res = doBearer(http.MethodPost, "/v2/project", ct, body.String(), tokOK)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("pat upload: %d", res.StatusCode)
	}
	ct, body = multipartUpload(t, pd, "pat2.jar", []byte("y"))
	res = doBearer(http.MethodPost, "/v2/project", ct, body.String(), tokRO)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("readonly pat upload: %d", res.StatusCode)
	}

	// owner 删除自己项目 → 204
	res = do(http.MethodDelete, "/v2/project/"+proj.ID, "", "", "sess-owner")
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("owner delete: %d", res.StatusCode)
	}
	if _, err := store.GetProject(ctx, proj.ID); err != ErrNotFound {
		t.Fatalf("project still exists: %v", err)
	}
}
