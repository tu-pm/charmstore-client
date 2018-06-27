package charmcmd_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"time"

	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/juju/loggo"
)

func newDockerHandler() *dockerHandler {
	return &dockerHandler{}
}

type pushRequest struct {
	imageID string
	tag     string
}

type tagRequest struct {
	imageID string
	tag     string
	repo    string
}

type deleteRequest struct {
	imageID string
}

type pullRequest struct {
	imageID string
	tag     string
}

type dockerHandler struct {
	mu   sync.Mutex
	reqs []interface{}
}

func (srv *dockerHandler) imageDigest(imageName string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(imageName)))
}

var logger = loggo.GetLogger("charm.cmd.charmtest")

func (srv *dockerHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Infof("dockerHandler.ServeHTTP %s %v", req.Method, req.URL)
	req.ParseForm()
	if !strings.HasPrefix(req.URL.Path, "/v1.12/images/") {
		http.NotFound(w, req)
		return
	}
	if req.Method == "DELETE" {
		srv.serveImageDelete(w, req)
		return
	}
	switch {
	case strings.HasSuffix(req.URL.Path, "/push"):
		srv.servePush(w, req)
	case strings.HasSuffix(req.URL.Path, "/create"):
		srv.servePull(w, req)
	case strings.HasSuffix(req.URL.Path, "/tag"):
		srv.serveTag(w, req)
	default:
		logger.Errorf("docker server page %q not found", req.URL)
		http.NotFound(w, req)
	}
}

func (srv *dockerHandler) serveImageDelete(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/v1.12/images/")
	srv.addRequest(deleteRequest{
		imageID: path,
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write([]byte("[]"))
}

func (srv *dockerHandler) serveTag(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/v1.12/images/")
	path = strings.TrimSuffix(path, "/tag")
	srv.addRequest(tagRequest{
		tag:     req.Form.Get("tag"),
		repo:    req.Form.Get("repo"),
		imageID: path,
	})
}

func (srv *dockerHandler) servePush(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/v1.12/images/")
	path = strings.TrimSuffix(path, "/push")
	// TODO include authentication creds in pushRequest?
	srv.addRequest(pushRequest{
		imageID: path,
		tag:     req.Form.Get("tag"),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	type finalStatus struct {
		Tag    string
		Digest string
		Size   int64
	}
	aux, _ := json.Marshal(finalStatus{
		Tag:    "latest",
		Digest: srv.imageDigest(path),
		Size:   10000,
	})
	auxMsg := json.RawMessage(aux)
	enc := json.NewEncoder(w)
	enc.Encode(jsonmessage.JSONMessage{
		Aux: &auxMsg,
	})
}

func (srv *dockerHandler) servePull(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/v1.12/images/create" {
		http.NotFound(w, req)
		return
	}
	srv.addRequest(pullRequest{
		imageID: req.Form.Get("fromImage"),
		tag:     req.Form.Get("tag"),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
}

func (srv *dockerHandler) addRequest(req interface{}) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.reqs = append(srv.reqs, req)
}

func newDockerRegistryHandler(authHandlerURL string) *dockerRegistryHandler {
	return &dockerRegistryHandler{
		apiVersion:     "registry/2.0",
		authHandlerURL: authHandlerURL,
	}
}

type registeredImage struct {
	version1 bool
	name     string
	digest   string
}

type dockerRegistryHandler struct {
	apiVersion     string
	authHandlerURL string
	images         []*registeredImage
	errors         []string
}

func (srv *dockerRegistryHandler) addImage(img *registeredImage) {
	srv.images = append(srv.images, img)
}

func (srv *dockerRegistryHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Infof("dockerRegistryHandler.ServeHTTP %s %v", req.Method, req.URL)
	if req.Method != "GET" && req.Method != "HEAD" {
		srv.addErrorf("unexpected method")
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		return
	}
	req.ParseForm()
	if req.URL.Path == "/v2/" {
		srv.serveV2Root(w, req)
		return
	}
	parts := strings.Split(req.URL.Path, "/")
	if len(parts) < 4 || parts[1] != "v2" || parts[len(parts)-2] != "manifests" {
		srv.addErrorf("unexpected access to path %v", req.URL)
		http.NotFound(w, req)
		return
	}
	if req.Header.Get("Accept") != "application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest-list.v2+json" {
		srv.addErrorf("expected Accept header not found")
		http.NotFound(w, req)
		return
	}
	imageName := strings.Join(parts[2:len(parts)-2], "/")
	tagOrDigest := parts[len(parts)-1]
	img := srv.findImage(imageName, tagOrDigest)
	if img == nil {
		http.NotFound(w, req)
		return
	}
	digest := img.digest
	if img.version1 {
		// It's a version 1 image but the client has requested
		// a version 2 manifest, so return a different digest.
		digest = sha256Digest(img.digest)
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Docker-Distribution-Api-Version", srv.apiVersion)
	// Don't bother with the manifest content, as the code
	// just discards it.
}

func (srv *dockerRegistryHandler) findImage(name, tag string) *registeredImage {
	for _, img := range srv.images {
		if img.name != name {
			continue
		}
		if img.digest == tag || tag == "latest" {
			return img
		}
	}
	return nil
}

func (srv *dockerRegistryHandler) serveV2Root(w http.ResponseWriter, req *http.Request) {
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service=\"registry.example.com\"`, srv.authHandlerURL))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !strings.HasPrefix(authHeader, "Bearer") {
		http.Error(w, "no bearer token", http.StatusBadRequest)
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != "sometoken" {
		http.Error(w, "unexpected token", 500)
	}
}

func (srv *dockerRegistryHandler) addErrorf(f string, a ...interface{}) {
	srv.errors = append(srv.errors, fmt.Sprintf(f, a...))
}

func newDockerAuthHandler() *dockerAuthHandler {
	return &dockerAuthHandler{}
}

type dockerAuthHandler struct{}

type tokenResp struct {
	Token     string    `json:"token"`
	ExpiresIn int       `json:"expires_in"`
	IssuedAt  time.Time `json:"issued_at"`
}

func (srv *dockerAuthHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Infof("dockerAuthHandler.ServeHTTP %s %v", req.Method, req.URL)
	req.ParseForm()
	if req.URL.Path != "/token" {
		http.Error(w, "unexpected call to docker auth handler", 500)
		return
	}
	token, _ := json.Marshal(tokenResp{
		Token:     "sometoken",
		ExpiresIn: 5000,
		IssuedAt:  time.Now(),
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write(token)
}

func sha256Digest(s string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(s)))
}
