package maxmind

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/laincat/GeoIP/lib"
	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/oschwald/geoip2-golang/v2"
)

const (
	TypeGeoLite2CountryMMDBOut = "maxmindMMDB"
	DescGeoLite2CountryMMDBOut = "Convert data to MaxMind mmdb database format"
)

func init() {
	lib.RegisterOutputConfigCreator(TypeGeoLite2CountryMMDBOut, func(action lib.Action, data json.RawMessage) (lib.OutputConverter, error) {
		return newGeoLite2CountryMMDBOut(TypeGeoLite2CountryMMDBOut, DescGeoLite2CountryMMDBOut, action, data)
	})
	lib.RegisterOutputConverter(TypeGeoLite2CountryMMDBOut, &GeoLite2CountryMMDBOut{
		Description: DescGeoLite2CountryMMDBOut,
	})
}

type GeoLite2CountryMMDBOut struct {
	Type        string
	Action      lib.Action
	Description string
	OutputName  string
	OutputDir   string
	Want        []string
	Overwrite   []string
	Exclude     []string
	OnlyIPType  lib.IPType

	SourceMMDBURI string
}

func (g *GeoLite2CountryMMDBOut) GetType() string {
	return g.Type
}

func (g *GeoLite2CountryMMDBOut) GetAction() lib.Action {
	return g.Action
}

func (g *GeoLite2CountryMMDBOut) GetDescription() string {
	return g.Description
}

func (g *GeoLite2CountryMMDBOut) Output(container lib.Container) error {
	// 元数据刻意保持 GeoLite2-Country 的形状：客户端读这个 mmdb 做 GEOIP 匹配时
	// 会校验 database_type / languages，沿用既有取值才不会被判成未知数据库。
	// 本项目不依赖 MaxMind 的任何数据或许可，这里只是沿用它的格式约定。
	writer, err := mmdbwriter.New(
		mmdbwriter.Options{
			DatabaseType:            "GeoLite2-Country",
			Description:             map[string]string{"en": "Customized GeoLite2 Country database"},
			Languages:               []string{"de", "en", "es", "fr", "ja", "pt-BR", "ru", "zh-CN"},
			RecordSize:              24,
			IncludeReservedNetworks: true,

			// Pinned via SOURCE_DATE_EPOCH so that identical inputs produce a
			// bit-for-bit identical artifact. 0 falls back to the current time.
			BuildEpoch: lib.BuildEpoch(),
		},
	)
	if err != nil {
		return err
	}

	if g.OutputName == defaultGeoLite2CountryMMDBOutputName {
		log.Printf("ℹ️ [type %s] build_epoch -> %s\n", g.Type, lib.BuildEpochString())
	}

	// Get extra info
	extraInfo, err := g.GetExtraInfo()
	if err != nil {
		return err
	}

	updated := false
	for _, name := range g.filterAndSortList(container) {
		entry, found := container.GetEntry(name)
		if !found {
			log.Printf("❌ entry %s not found\n", name)
			continue
		}

		if err := g.marshalData(writer, entry, extraInfo); err != nil {
			return err
		}

		updated = true
	}

	if updated {
		return g.writeFile(g.OutputName, writer)
	}

	return nil
}

func (g *GeoLite2CountryMMDBOut) filterAndSortList(container lib.Container) []string {
	/*
		Note: The IPs and/or CIDRs of the latter list will overwrite those of the former one
		when duplicated data found due to MaxMind mmdb file format constraint.

		Be sure to place the name of the most important list at last
		when writing wantedList and overwriteList in config file.

		The order of names in wantedList has a higher priority than which of the overwriteList.
	*/

	excludeMap := make(map[string]bool)
	for _, exclude := range g.Exclude {
		if exclude = strings.ToUpper(strings.TrimSpace(exclude)); exclude != "" {
			excludeMap[exclude] = true
		}
	}

	wantList := make([]string, 0, len(g.Want))
	for _, want := range g.Want {
		if want = strings.ToUpper(strings.TrimSpace(want)); want != "" && !excludeMap[want] {
			wantList = append(wantList, want)
		}
	}

	if len(wantList) > 0 {
		return wantList
	}

	overwriteList := make([]string, 0, len(g.Overwrite))
	overwriteMap := make(map[string]bool)
	for _, overwrite := range g.Overwrite {
		if overwrite = strings.ToUpper(strings.TrimSpace(overwrite)); overwrite != "" && !excludeMap[overwrite] {
			overwriteList = append(overwriteList, overwrite)
			overwriteMap[overwrite] = true
		}
	}

	list := make([]string, 0, 300)
	for entry := range container.Loop() {
		name := entry.GetName()
		if excludeMap[name] || overwriteMap[name] {
			continue
		}
		list = append(list, name)
	}

	// Sort the lists
	slices.Sort(list)

	// Make sure the names in overwriteList are written at last
	list = append(list, overwriteList...)

	return list
}

func (g *GeoLite2CountryMMDBOut) marshalData(writer *mmdbwriter.Tree, entry *lib.Entry, extraInfo map[string]any) error {
	entryCidr, err := entry.MarshalText(lib.GetIgnoreIPType(g.OnlyIPType))
	if err != nil {
		return err
	}

	var record mmdbtype.DataType
	switch strings.TrimSpace(g.SourceMMDBURI) {
	case "": // No need to get extra info
		// 只落 iso_code —— 产物因此不含任何需要授权分发的 MaxMind 文本数据。
		record = mmdbtype.Map{
			"country": mmdbtype.Map{
				"iso_code": mmdbtype.String(entry.GetName()),
			},
		}

	default: // Get extra info
		info, found := extraInfo[entry.GetName()].(geoip2.Country)
		if !found {
			log.Printf("⚠️ [type %s | action %s] not found extra info for list %s\n", g.Type, g.Action, entry.GetName())

			record = mmdbtype.Map{
				"country": mmdbtype.Map{
					"iso_code": mmdbtype.String(entry.GetName()),
				},
			}
		} else if info.Continent.Code != "" {
			record = mmdbtype.Map{
				"continent": mmdbtype.Map{
					"names": mmdbtype.Map{
						"de":    mmdbtype.String(info.Continent.Names.German),
						"en":    mmdbtype.String(info.Continent.Names.English),
						"es":    mmdbtype.String(info.Continent.Names.Spanish),
						"fr":    mmdbtype.String(info.Continent.Names.French),
						"ja":    mmdbtype.String(info.Continent.Names.Japanese),
						"pt-BR": mmdbtype.String(info.Continent.Names.BrazilianPortuguese),
						"ru":    mmdbtype.String(info.Continent.Names.Russian),
						"zh-CN": mmdbtype.String(info.Continent.Names.SimplifiedChinese),
					},
					"code":       mmdbtype.String(info.Continent.Code),
					"geoname_id": mmdbtype.Uint32(info.Continent.GeoNameID),
				},
				"country": mmdbtype.Map{
					"names": mmdbtype.Map{
						"de":    mmdbtype.String(info.Country.Names.German),
						"en":    mmdbtype.String(info.Country.Names.English),
						"es":    mmdbtype.String(info.Country.Names.Spanish),
						"fr":    mmdbtype.String(info.Country.Names.French),
						"ja":    mmdbtype.String(info.Country.Names.Japanese),
						"pt-BR": mmdbtype.String(info.Country.Names.BrazilianPortuguese),
						"ru":    mmdbtype.String(info.Country.Names.Russian),
						"zh-CN": mmdbtype.String(info.Country.Names.SimplifiedChinese),
					},
					"iso_code":             mmdbtype.String(entry.GetName()),
					"geoname_id":           mmdbtype.Uint32(info.Country.GeoNameID),
					"is_in_european_union": mmdbtype.Bool(info.Country.IsInEuropeanUnion),
				},
			}
		} else {
			record = mmdbtype.Map{
				"country": mmdbtype.Map{
					"names": mmdbtype.Map{
						"de":    mmdbtype.String(info.Country.Names.German),
						"en":    mmdbtype.String(info.Country.Names.English),
						"es":    mmdbtype.String(info.Country.Names.Spanish),
						"fr":    mmdbtype.String(info.Country.Names.French),
						"ja":    mmdbtype.String(info.Country.Names.Japanese),
						"pt-BR": mmdbtype.String(info.Country.Names.BrazilianPortuguese),
						"ru":    mmdbtype.String(info.Country.Names.Russian),
						"zh-CN": mmdbtype.String(info.Country.Names.SimplifiedChinese),
					},
					"iso_code":             mmdbtype.String(entry.GetName()),
					"geoname_id":           mmdbtype.Uint32(info.Country.GeoNameID),
					"is_in_european_union": mmdbtype.Bool(info.Country.IsInEuropeanUnion),
				},
			}
		}

	}

	for _, cidr := range entryCidr {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return err
		}
		if err := writer.Insert(network, record); err != nil {
			return err
		}
	}

	return nil
}

func (g *GeoLite2CountryMMDBOut) writeFile(filename string, writer *mmdbwriter.Tree) error {
	if err := os.MkdirAll(g.OutputDir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(g.OutputDir, filename), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = writer.WriteTo(f)
	if err != nil {
		return err
	}

	log.Printf("✅ [%s] %s --> %s", g.Type, filename, g.OutputDir)

	return nil
}
