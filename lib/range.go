package lib

import (
	"compress/gzip"
	"io"
	"net/netip"
	"os"
	"strings"

	"go4.org/netipx"
)

// 本文件收纳「按区间喂数据」的公共工具。
//
// 上游的输入插件多以 CIDR 行为单位读取数据，而 IPInfo / iptoasn 这类
// 以「起止 IP」成对给出的转储，逐行拆 CIDR 是纯粹的浪费。这里把区间写入
// 与批量提交的样板收拢到一处，让各个区间型输入插件只关心自己的解析逻辑。

// AddIPRangeToEntries 按名字取（不存在则新建）条目，并并入一段地址。
func AddIPRangeToEntries(entries map[string]*Entry, name string, ipRange netipx.IPRange) error {
	entry, found := entries[name]
	if !found {
		entry = NewEntry(name)
		entries[name] = entry
	}

	return entry.AddIPRange(ipRange)
}

// ApplyEntries 把一整批条目按 action 提交到容器。
func ApplyEntries(container Container, entries map[string]*Entry, action Action, onlyIPType IPType) (Container, error) {
	ignoreIPType := GetIgnoreIPType(onlyIPType)

	for _, entry := range entries {
		switch action {
		case ActionAdd:
			if err := container.Add(entry, ignoreIPType); err != nil {
				return nil, err
			}
		case ActionRemove:
			if err := container.Remove(entry, CaseRemovePrefix, ignoreIPType); err != nil {
				return nil, err
			}
		default:
			return nil, ErrUnknownAction
		}
	}

	return container, nil
}

// ParseAddrString 解析单个 IP，并统一归一化掉 IPv4-mapped-IPv6 形式，
// 避免同一段地址在 v4 与 v6 两套 builder 里各存一份。
func ParseAddrString(raw string) (netip.Addr, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, false
	}

	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false
	}

	return addr.Unmap(), true
}

// NormalizeASN 把 "AS13335" / "as13335" / "13335" 统一成裸数字 "13335"。
// 各家数据源写法不一：IPInfo 的 CSV 用裸数字，配置里人手写时习惯带 AS 前缀。
func NormalizeASN(asn string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asn)), "as")
}

// SkipByIPType 判断某段地址是否需要被 onlyIPType 过滤掉。
// 提前跳过可以少建一套 builder，对百万行级的转储是实打实的内存节省。
func SkipByIPType(ipRange netipx.IPRange, onlyIPType IPType) bool {
	switch onlyIPType {
	case IPv4:
		return !ipRange.From().Is4()
	case IPv6:
		return !ipRange.From().Is6()
	default:
		return false
	}
}

// gzipReadCloser 让压缩流与底层流一起被关闭，否则 HTTP 连接不会被释放。
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
func OpenMaybeGzip(uri string) (io.ReadCloser, error) {
	var raw io.ReadCloser

	switch {
	case strings.HasPrefix(strings.ToLower(uri), "http://"), strings.HasPrefix(strings.ToLower(uri), "https://"):
		remote, err := GetRemoteURLReader(uri)
		if err != nil {
			return nil, err
		}
		raw = remote

	default:
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
