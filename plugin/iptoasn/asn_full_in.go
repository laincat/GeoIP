package iptoasn

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/laincat/GeoIP/lib"
	"go4.org/netipx"
)

// 全量 ASN 输入：与 ipToASN 的区别在于不做 wantedList 过滤，而是把转储里
// 的每一个 ASN 都建一个条目（name = AS<数字>），组织名挂到条目元数据里，
// 供 maxmind 的 ASN 输出插件落成 GeoLite2-ASN 兼容的 mmdb。
//
// iptoasn 全集约 10 万个 ASN，条目数量级是 Country.mmdb 的国家维度所不能比的，
// 因此单独成一条输入，只在需要 ASN 产物时启用，别并进 Country.mmdb 的构建。
const (
	TypeIPToASNFull = "ipToASNFull"
	DescIPToASNFull = "Import full iptoasn.com ASN dump with org names"
)

func init() {
	lib.RegisterInputConfigCreator(TypeIPToASNFull, func(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
		return newIPToASNFullIn(action, data)
	})
	lib.RegisterInputConverter(TypeIPToASNFull, &IPToASNFullIn{
		Description: DescIPToASNFull,
	})
}

func newIPToASNFullIn(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
	var tmp struct {
		IPv4File   string     `json:"ipv4"`
		IPv6File   string     `json:"ipv6"`
		OnlyIPType lib.IPType `json:"onlyIPType"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(tmp.IPv4File) == "" && strings.TrimSpace(tmp.IPv6File) == "" {
		tmp.IPv4File = defaultIPv4File
		tmp.IPv6File = defaultIPv6File
	}

	return &IPToASNFullIn{
		Type:        TypeIPToASNFull,
		Action:      action,
		Description: DescIPToASNFull,
		IPv4File:    tmp.IPv4File,
		IPv6File:    tmp.IPv6File,
		OnlyIPType:  tmp.OnlyIPType,
	}, nil
}

type IPToASNFullIn struct {
	Type        string
	Action      lib.Action
	Description string
	IPv4File    string
	IPv6File    string
	OnlyIPType  lib.IPType
}

func (g *IPToASNFullIn) GetType() string {
	return g.Type
}

func (g *IPToASNFullIn) GetAction() lib.Action {
	return g.Action
}

func (g *IPToASNFullIn) GetDescription() string {
	return g.Description
}

func (g *IPToASNFullIn) Input(container lib.Container) (lib.Container, error) {
	entries := make(map[string]*lib.Entry)

	if g.IPv4File != "" {
		if err := g.process(g.IPv4File, entries); err != nil {
			return nil, err
		}
	}

	if g.IPv6File != "" {
		if err := g.process(g.IPv6File, entries); err != nil {
			return nil, err
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("❌ [type %s | action %s] no entry is generated", g.Type, g.Action)
	}

	return lib.ApplyEntries(container, entries, g.Action, g.OnlyIPType)
}

func (g *IPToASNFullIn) process(file string, entries map[string]*lib.Entry) error {
	rc, err := lib.OpenMaybeGzip(file)
	if err != nil {
		return err
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	matched := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 3 {
			continue
		}

		asn := strings.TrimSpace(fields[2])
		if asn == "" || asn == "0" {
			continue
		}

		start, ok := lib.ParseAddrString(fields[0])
		if !ok {
			continue
		}
		end, ok := lib.ParseAddrString(fields[1])
		if !ok {
			continue
		}

		ipRange := netipx.IPRangeFrom(start, end)
		if !ipRange.IsValid() || lib.SkipByIPType(ipRange, g.OnlyIPType) {
			continue
		}

		name := "AS" + asn
		entry, found := entries[name]
		if !found {
			entry = lib.NewEntry(name)
			entries[name] = entry
		}

		// 组织名在 TSV 第 5 列；某些行可能缺列，此时退化为空字符串。
		org := ""
		if len(fields) >= 5 {
			org = strings.TrimSpace(fields[4])
		}
		if _, ok := entry.GetExtra("asn_org"); !ok {
			entry.SetExtra("asn_org", org)
		}

		if err := entry.AddIPRange(ipRange); err != nil {
			return err
		}
		matched++
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if matched == 0 {
		return fmt.Errorf("❌ [type %s | action %s] %s matched no ASN", g.Type, g.Action, file)
	}

	return nil
}