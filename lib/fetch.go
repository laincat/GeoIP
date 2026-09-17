package lib

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// 本文件收纳「把一份数据取进来」的全部细节：远端 HTTP、本地文件、透明解压。
//
// 上游把这些逻辑散在每个插件里 —— 每个输入插件各写一遍 `http.Get` +
// 各自判断状态码。后果是「加超时」这种改动要改 N 遍，而且必然漏掉几处
// （`plugin/plaintext/text_in.go` 就漏了）。这里收敛成一套，所有插件共用。

const (
	// 为什么必须有超时：http.Client 的零值**没有超时**，一条卡住的连接
	// 会把整个 CI 挂到 6 小时上限才被 runner 杀掉。GitHub Raw / gstatic /
	// AWS 这些源偶发长时间不返回，属于必须防御的情况。
	httpTimeout = 5 * time.Minute
	// 重试次数与间隔。CI 里网络抖动是常态，一次失败不该毁掉一整轮构建。
	httpMaxAttempts = 3
	httpRetryDelay  = 3 * time.Second
	// 固定 UA。空 UA 会被一部分前置了 Cloudflare 的站点直接 403。
	httpUserAgent = "laincat-GeoIP/1.0 (+https://github.com/laincat/GeoIP)"
)

var httpClient = &http.Client{Timeout: httpTimeout}

// GetRemoteURLReader 打开远端 URL 并返回响应体，由调用方负责 Close。
//
// 重试策略：网络错误与 5xx 重试到 httpMaxAttempts 次；
// 4xx 立即失败 —— 地址写错了重试多少次都一样。
func GetRemoteURLReader(url string) (io.ReadCloser, error) {
	var lastErr error

	for attempt := 1; attempt <= httpMaxAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", httpUserAgent)

		resp, err := httpClient.Do(req)
		switch {
		case err != nil:
			lastErr = err

		case resp.StatusCode == http.StatusOK:
			return resp.Body, nil

		default:
			resp.Body.Close()
			lastErr = fmt.Errorf("failed to get remote content -> %s: %s", url, resp.Status)
			if resp.StatusCode < http.StatusInternalServerError {
				return nil, lastErr
			}
		}

		if attempt < httpMaxAttempts {
			time.Sleep(httpRetryDelay)
		}
	}

	return nil, lastErr
}

// GetRemoteURLContent 把远端内容整个读进内存。
// 大文件（IPInfo 的 11 MB CSV、iptoasn 的 9 MB 转储）请改用
// OpenMaybeGzip 走流式，别走这里。
func GetRemoteURLContent(url string) ([]byte, error) {
	rc, err := GetRemoteURLReader(url)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

// gzipReadCloser 让压缩流与底层流一起被关闭，否则连接不会被释放。
type gzipReadCloser struct {
	io.Reader
	closers []io.Closer
}

func (r *gzipReadCloser) Close() error {
	var firstErr error
	for _, closer := range r.closers {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// OpenMaybeGzip 打开本地路径或 HTTP(S) 地址，并在以 .gz 结尾时透明解压。
//
// 这是各输入插件的统一入口：`uri` 既能是 ci 里下载好的本地文件，
// 也能直接是远端地址，调用方不用关心是哪种。
func OpenMaybeGzip(uri string) (io.ReadCloser, error) {
	var raw io.ReadCloser

	if IsRemoteURI(uri) {
		remote, err := GetRemoteURLReader(uri)
		if err != nil {
			return nil, err
		}
		raw = remote
	} else {
		file, err := os.Open(uri)
		if err != nil {
			return nil, err
		}
		raw = file
	}

	if !strings.HasSuffix(strings.ToLower(uri), ".gz") {
		return raw, nil
	}

	gz, err := gzip.NewReader(raw)
	if err != nil {
		raw.Close()
		return nil, err
	}

	return &gzipReadCloser{Reader: gz, closers: []io.Closer{gz, raw}}, nil
}

// IsRemoteURI 判断 uri 是远端地址还是本地路径。
// 集中一处，避免每个插件各写一遍前缀比较（上游就有 5 处重复）。
func IsRemoteURI(uri string) bool {
	lower := strings.ToLower(strings.TrimSpace(uri))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
