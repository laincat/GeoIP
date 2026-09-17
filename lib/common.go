package lib

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// 本文件放「不属于任何单一职责」的公共助手。
// 取数（HTTP / 本地文件 / gzip）已拆到 fetch.go；区间工具在 range.go。

// GetIgnoreIPType 把 onlyIPType 翻译成「要忽略哪种 IP」。
func GetIgnoreIPType(onlyIPType IPType) IgnoreIPOption {
	switch onlyIPType {
	case IPv4:
		return IgnoreIPv6
	case IPv6:
		return IgnoreIPv4
	}

	return nil
}

// WantedListExtended 让同一份配置既能写数组也能写 map：
//
//	"wantedList": ["cn", "us"]
//	"wantedList": { "google": ["AS15169", …] }
//
// 两种形态语义不同 —— 数组是「要哪些名字」，map 是「类别 -> 该类别包含什么」。
type WantedListExtended struct {
	TypeSlice []string
	TypeMap   map[string][]string
}

func (w *WantedListExtended) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	slice := make([]string, 0)
	mapMap := make(map[string][]string)

	if err := json.Unmarshal(data, &slice); err != nil {
		if err2 := json.Unmarshal(data, &mapMap); err2 != nil {
			return err2
		}
	}

	w.TypeSlice = slice
	w.TypeMap = mapMap

	return nil
}

// BuildEpoch returns the Unix timestamp that is written into the build_epoch
// field of a generated MaxMind MMDB file.
//
// mmdbwriter defaults that field to time.Now().Unix(), which means two builds
// from identical inputs still differ by a few bytes and therefore never share
// a checksum. Honouring the SOURCE_DATE_EPOCH environment variable (the
// reproducible-builds.org convention) pins the value, so that the same input
// tree always produces a bit-for-bit identical artifact.
//
// The repository CI sets SOURCE_DATE_EPOCH to the committer timestamp of the
// revision being built. When the variable is absent or unusable the function
// returns 0, which tells mmdbwriter to keep using time.Now().Unix().
func BuildEpoch() int64 {
	epoch, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("SOURCE_DATE_EPOCH")), 10, 64)
	if err != nil || epoch <= 0 {
		return 0
	}

	return epoch
}

// BuildEpochString renders BuildEpoch() for human-readable log output.
func BuildEpochString() string {
	epoch := BuildEpoch()
	if epoch == 0 {
		return "current time"
	}

	return fmt.Sprintf("%d (%s UTC)", epoch, time.Unix(epoch, 0).UTC().Format(time.RFC3339))
}
