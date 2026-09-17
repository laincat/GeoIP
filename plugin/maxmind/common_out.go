package maxmind

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laincat/GeoIP/lib"
	"github.com/oschwald/geoip2-golang/v2"
	"github.com/oschwald/maxminddb-golang/v2"
)

var (
	// 产物默认文件名。注意 mmdb 的 database_type 仍是 GeoLite2-Country：
	// 客户端（Surge / Clash / sing-box）读 GEOIP 时会校验这个字符串，
	// 换成自造名字会让产物不被识别，所以只在文件名上做收敛。
	defaultGeoLite2CountryMMDBOutputName = "Country.mmdb"

	defaultMaxmindOutputDir = filepath.Join("./", "output", "maxmind")
)

// newGeoLite2CountryMMDBOut 解析 output 段里的 args。
//
// 上游按 iType 分三路（maxmindMMDB / dbipCountryMMDB / ipinfoCountryMMDB）。
// 本项目只保留 mmdb 这一种输出形态，对应的 dbip / ipinfo 两个插件已删除，
// 于是三路分支收敛成一路。
func newGeoLite2CountryMMDBOut(iType string, iDesc string, action lib.Action, data json.RawMessage) (lib.OutputConverter, error) {
	var tmp struct {
		OutputName string     `json:"outputName"`
		OutputDir  string     `json:"outputDir"`
		Want       []string   `json:"wantedList"`
		Overwrite  []string   `json:"overwriteList"`
		Exclude    []string   `json:"excludedList"`
		OnlyIPType lib.IPType `json:"onlyIPType"`

		SourceMMDBURI string `json:"sourceMMDBURI"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if tmp.OutputName == "" {
		tmp.OutputName = defaultGeoLite2CountryMMDBOutputName
	}

	if tmp.OutputDir == "" {
		tmp.OutputDir = defaultMaxmindOutputDir
	}

	return &GeoLite2CountryMMDBOut{
		Type:        iType,
		Action:      action,
		Description: iDesc,
		OutputName:  tmp.OutputName,
		OutputDir:   tmp.OutputDir,
		Want:        tmp.Want,
		Overwrite:   tmp.Overwrite,
		Exclude:     tmp.Exclude,
		OnlyIPType:  tmp.OnlyIPType,

		SourceMMDBURI: tmp.SourceMMDBURI,
	}, nil
}

// GetExtraInfo 从 sourceMMDBURI 指向的既有 mmdb 里抽出国家维度的附加字段
// （国家名、大洲、geoname_id、是否欧盟），供 marshalData 拼进产物。
//
// 没配 sourceMMDBURI 时立刻返回 nil —— 本仓库的默认构建正是这条路径，
// 产物只带 iso_code，不带任何需要授权分发的 MaxMind 文本数据。
func (g *GeoLite2CountryMMDBOut) GetExtraInfo() (map[string]any, error) {
	if strings.TrimSpace(g.SourceMMDBURI) == "" {
		return nil, nil
	}

	var content []byte
	var err error
	if lib.IsRemoteURI(g.SourceMMDBURI) {
		content, err = lib.GetRemoteURLContent(g.SourceMMDBURI)
	} else {
		content, err = os.ReadFile(g.SourceMMDBURI)
	}
	if err != nil {
		return nil, err
	}

	db, err := maxminddb.OpenBytes(content)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	infoList := make(map[string]any)
	for network := range db.Networks() {
		var record geoip2.Country
		if err := network.Decode(&record); err != nil {
			return nil, err
		}

		switch {
		case strings.TrimSpace(record.Country.ISOCode) != "":
			countryCode := strings.ToUpper(strings.TrimSpace(record.Country.ISOCode))
			if _, found := infoList[countryCode]; !found {
				infoList[countryCode] = geoip2.Country{
					Continent: record.Continent,
					Country:   record.Country,
				}
			}

		case strings.TrimSpace(record.RegisteredCountry.ISOCode) != "":
			countryCode := strings.ToUpper(strings.TrimSpace(record.RegisteredCountry.ISOCode))
			if _, found := infoList[countryCode]; !found {
				infoList[countryCode] = geoip2.Country{
					Continent: record.Continent,
					Country:   record.RegisteredCountry,
				}
			}

		case strings.TrimSpace(record.RepresentedCountry.ISOCode) != "":
			countryCode := strings.ToUpper(strings.TrimSpace(record.RepresentedCountry.ISOCode))
			if _, found := infoList[countryCode]; !found {
				infoList[countryCode] = geoip2.Country{
					Continent: record.Continent,
					Country: geoip2.CountryRecord{
						Names:             record.RepresentedCountry.Names,
						ISOCode:           record.RepresentedCountry.ISOCode,
						GeoNameID:         record.RepresentedCountry.GeoNameID,
						IsInEuropeanUnion: record.RepresentedCountry.IsInEuropeanUnion,
					},
				}
			}
		}
	}

	if len(infoList) == 0 {
		return nil, fmt.Errorf("❌ [type %s | action %s] no extra info found in the source MMDB file: %s", g.Type, g.Action, g.SourceMMDBURI)
	}

	return infoList, nil
}
