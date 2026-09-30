package maxmind

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/laincat/GeoIP/lib"
	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// GeoLite2-ASN 兼容的 mmdb 输出。mihomo 读 ASN 时对 DatabaseType 有硬要求：
// 只认 "GeoLite2-ASN"（或 DBIP 的 compat 别名），record 结构固定为
// autonomous_system_number (uint32) + autonomous_system_organization (string)。
// 因此这里必须逐字沿用这个形状，换成自造字段名客户端就读不出来。
const (
	TypeASNMMDBOut = "asnMMDB"
	DescASNMMDBOut = "Convert ASN entries to GeoLite2-ASN compatible mmdb"
)

var defaultASNMMDBOutputName = "GeoLite2-ASN.mmdb"

func init() {
	lib.RegisterOutputConfigCreator(TypeASNMMDBOut, func(action lib.Action, data json.RawMessage) (lib.OutputConverter, error) {
		return newASNMMDBOut(action, data)
	})
	lib.RegisterOutputConverter(TypeASNMMDBOut, &ASNMMDBOut{
		Description: DescASNMMDBOut,
	})
}

func newASNMMDBOut(action lib.Action, data json.RawMessage) (lib.OutputConverter, error) {
	var tmp struct {
		OutputName string     `json:"outputName"`
		OutputDir  string     `json:"outputDir"`
		Exclude    []string   `json:"excludedList"`
		OnlyIPType lib.IPType `json:"onlyIPType"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if tmp.OutputName == "" {
		tmp.OutputName = defaultASNMMDBOutputName
	}
	if tmp.OutputDir == "" {
		tmp.OutputDir = defaultMaxmindOutputDir
	}

	return &ASNMMDBOut{
		Type:        TypeASNMMDBOut,
		Action:      action,
		Description: DescASNMMDBOut,
		OutputName:  tmp.OutputName,
		OutputDir:   tmp.OutputDir,
		Exclude:     tmp.Exclude,
		OnlyIPType:  tmp.OnlyIPType,
	}, nil
}

type ASNMMDBOut struct {
	Type        string
	Action      lib.Action
	Description string
	OutputName  string
	OutputDir   string
	Exclude     []string
	OnlyIPType  lib.IPType
}

func (g *ASNMMDBOut) GetType() string {
	return g.Type
}

func (g *ASNMMDBOut) GetAction() lib.Action {
	return g.Action
}

func (g *ASNMMDBOut) GetDescription() string {
	return g.Description
}

func (g *ASNMMDBOut) Output(container lib.Container) error {
	writer, err := mmdbwriter.New(
		mmdbwriter.Options{
			DatabaseType:            "GeoLite2-ASN",
			Description:             map[string]string{"en": "iptoasn.com ASN database (GeoLite2-ASN compatible)"},
			Languages:               []string{"en"},
			RecordSize:              24,
			IncludeReservedNetworks: true,
			BuildEpoch:              lib.BuildEpoch(),
		},
	)
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

		if err := g.marshalData(writer, entry); err != nil {
			return err
		}
		updated = true
	}

	if !updated {
		return nil
	}

	if err := os.MkdirAll(g.OutputDir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(g.OutputDir, g.OutputName), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := writer.WriteTo(f); err != nil {
		return err
	}

	log.Printf("✅ [%s] %s --> %s", g.Type, g.OutputName, g.OutputDir)
	return nil
}

func (g *ASNMMDBOut) filterAndSortList(container lib.Container) []string {
	excludeMap := make(map[string]bool)
	for _, exclude := range g.Exclude {
		if exclude = strings.ToUpper(strings.TrimSpace(exclude)); exclude != "" {
			excludeMap[exclude] = true
		}
	}

	list := make([]string, 0, 300)
	for entry := range container.Loop() {
		name := entry.GetName()
		if excludeMap[name] {
			continue
		}
		list = append(list, name)
	}

	slices.Sort(list)
	return list
}

func (g *ASNMMDBOut) marshalData(writer *mmdbwriter.Tree, entry *lib.Entry) error {
	entryCidr, err := entry.MarshalText(lib.GetIgnoreIPType(g.OnlyIPType))
	if err != nil {
		return err
	}

	// 条目名是 "AS13335"，ASN 号取后面的数字部分；组织名从元数据里取。
	asnStr := strings.TrimPrefix(strings.ToUpper(entry.GetName()), "AS")
	asnNum, err := strconv.ParseUint(asnStr, 10, 32)
	if err != nil {
		return fmt.Errorf("❌ [type %s | action %s] entry %s is not a valid ASN name", g.Type, g.Action, entry.GetName())
	}

	org, _ := entry.GetExtra("asn_org")
	record := mmdbtype.Map{
		"autonomous_system_number":       mmdbtype.Uint32(uint32(asnNum)),
		"autonomous_system_organization": mmdbtype.String(org),
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