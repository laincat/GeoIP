package lib

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func GetRemoteURLContent(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get remote content -> %s: %s", url, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func GetRemoteURLReader(url string) (io.ReadCloser, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to get remote content -> %s: %s", url, resp.Status)
	}

	return resp.Body, nil
}

func GetIgnoreIPType(onlyIPType IPType) IgnoreIPOption {
	switch onlyIPType {
	case IPv4:
		return IgnoreIPv6
	case IPv6:
		return IgnoreIPv4
	}

	return nil
}

type WantedListExtended struct {
	TypeSlice []string
	TypeMap   map[string][]string
}

func (w *WantedListExtended) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	slice := make([]string, 0)
	mapMap := make(map[string][]string, 0)

	err := json.Unmarshal(data, &slice)
	if err != nil {
		err2 := json.Unmarshal(data, &mapMap)
		if err2 != nil {
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
	raw := strings.TrimSpace(os.Getenv("SOURCE_DATE_EPOCH"))
	if raw == "" {
		return 0
	}

	epoch, err := strconv.ParseInt(raw, 10, 64)
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
