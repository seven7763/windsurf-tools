package services

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StaticCacheConfig controls static response cache interception.
type StaticCacheConfig struct {
	Enabled  bool
	CacheDir string
}

// ── Path → filename mapping ──

var staticPathMap = map[string]string{
	"GetUserStatus":          "GetUserStatus.bin",
	"GetModelStatuses":       "GetModelStatuses.bin",
	"GetCommandModelConfigs": "GetCommandModelConfigs.bin",
}

func staticCacheFileName(path string) string {
	for endpoint, filename := range staticPathMap {
		if strings.Contains(path, endpoint) {
			return filename
		}
	}
	return ""
}

func isStaticCacheEndpoint(path string) bool {
	return staticCacheFileName(path) != ""
}

// ── In-memory file cache with modification-time check ──

type staticEntry struct {
	data    []byte
	modTime time.Time
}

type staticFileCache struct {
	mu      sync.RWMutex
	entries map[string]*staticEntry
}

var staticCache = &staticFileCache{
	entries: make(map[string]*staticEntry),
}

// loadStaticFile loads a cached .bin file, using in-memory cache with mtime check.
// Replacing the file on disk takes effect immediately on the next request.
func loadStaticFile(dir, name string) ([]byte, bool) {
	fpath := filepath.Join(dir, name)
	info, err := os.Stat(fpath)
	if err != nil {
		return nil, false
	}

	staticCache.mu.RLock()
	if e, ok := staticCache.entries[name]; ok && e.modTime.Equal(info.ModTime()) {
		data := make([]byte, len(e.data))
		copy(data, e.data)
		staticCache.mu.RUnlock()
		return data, true
	}
	staticCache.mu.RUnlock()

	f, err := os.Open(fpath)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || len(data) == 0 {
		return nil, false
	}

	staticCache.mu.Lock()
	staticCache.entries[name] = &staticEntry{data: data, modTime: info.ModTime()}
	staticCache.mu.Unlock()
	return data, true
}

func invalidateStaticCache(name string) {
	staticCache.mu.Lock()
	delete(staticCache.entries, name)
	staticCache.mu.Unlock()
}

// ── Static cache interception ──

// tryServeStaticCache intercepts matching endpoints and returns cached .bin files
// directly, bypassing the upstream ReverseProxy. Returns true if handled.
func (p *MitmProxy) tryServeStaticCache(w http.ResponseWriter, r *http.Request) bool {
	p.mu.RLock()
	cfg := p.staticCacheConfig
	p.mu.RUnlock()

	if !cfg.Enabled || cfg.CacheDir == "" {
		return false
	}

	urlPath := r.URL.Path
	name := staticCacheFileName(urlPath)
	if name == "" {
		return false
	}

	data, ok := loadStaticFile(cfg.CacheDir, name)
	if !ok {
		return false
	}

	// GetUserStatus.bin + forge enabled → overlay forgeUserStatusResponse
	if strings.Contains(urlPath, "GetUserStatus") {
		p.mu.RLock()
		forgeCfg := p.forgeConfig
		p.mu.RUnlock()
		if forgeCfg.Enabled {
			data = forgeUserStatusResponse(data, forgeCfg)
		}
	}

	w.Header().Set("Content-Type", "application/proto")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	p.log("静态缓存命中: %s (%d bytes)", name, len(data))
	return true
}

// ── MitmProxy static cache configuration ──

// SetStaticCacheConfig updates the static cache configuration (thread-safe).
func (p *MitmProxy) SetStaticCacheConfig(cfg StaticCacheConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.staticCacheConfig = cfg
}

// GetStaticCacheConfig returns the current static cache configuration (thread-safe).
func (p *MitmProxy) GetStaticCacheConfig() StaticCacheConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.staticCacheConfig
}
