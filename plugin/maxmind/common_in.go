package maxmind

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/laincat/GeoIP/lib"
)

// 读既有 mmdb 作输入时的默认路径。
//
// 上游这里还有 db-ip 与 ipinfo 两个默认值，随对应的插件文件一起删了 ——
// 本项目只保留 mmdb 这一种输入形态。
var defaultGeoLite2CountryMMDBFile = filepath.Join("./", "geolite2", "GeoLite2-Country.mmdb")

func newGeoLite2CountryMMDBIn(iType string, iDesc string, action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
	var tmp struct {
		URI        string     `json:"uri"`
		Want       []string   `json:"wantedList"`
		OnlyIPType lib.IPType `json:"onlyIPType"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if tmp.URI == "" {
		tmp.URI = defaultGeoLite2CountryMMDBFile
	}

	// Filter want list
	wantList := make(map[string]bool)
	for _, want := range tmp.Want {
		if want = strings.ToUpper(strings.TrimSpace(want)); want != "" {
			wantList[want] = true
		}
	}

	return &GeoLite2CountryMMDBIn{
		Type:        iType,
		Action:      action,
		Description: iDesc,
		URI:         tmp.URI,
		Want:        wantList,
		OnlyIPType:  tmp.OnlyIPType,
	}, nil
}
