package iptoasn

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/laincat/GeoIP/lib"
	"go4.org/netipx"
)

const (
	TypeIPToASN = "ipToASN"
	DescIPToASN = "Convert iptoasn.com IP-to-ASN TSV data to other formats"
)

var (
	defaultIPv4File = filepath.Join("./", "iptoasn", "ip2asn-v4.tsv")
	defaultIPv6File = filepath.Join("./", "iptoasn", "ip2asn-v6.tsv")
)

// iptoasn.com 提供的是「起止 IP + ASN + 国家 + AS 名」的 5 列 TSV，无表头：
//
//	1.0.0.0	1.0.0.255	13335	US	CLOUDFLARENET
//	1.0.1.0	1.0.3.255	0	None	Not routed
//
// 它的定位与 MaxMind GeoLite2-ASN 相同，但无需 license key，可直接下载，
// 因此这里用它替代「按 ASN 归类」的那部分数据源。
//
// 参考：https://iptoasn.com/
func init() {
	lib.RegisterInputConfigCreator(TypeIPToASN, func(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
		return newIPToASNIn(action, data)
	})
	lib.RegisterInputConverter(TypeIPToASN, &IPToASNIn{
		Description: DescIPToASN,
	})
}

func newIPToASNIn(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
	var tmp struct {
		IPv4File   string                 `json:"ipv4"`
		IPv6File   string                 `json:"ipv6"`
		Want       lib.WantedListExtended `json:"wantedList"`
		OnlyIPType lib.IPType             `json:"onlyIPType"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	// 两个文件都没指定时，使用默认路径
	if strings.TrimSpace(tmp.IPv4File) == "" && strings.TrimSpace(tmp.IPv6File) == "" {
		tmp.IPv4File = defaultIPv4File
		tmp.IPv6File = defaultIPv6File
	}

	// ASN -> 类别名。map 形式是「类别 -> ASN 列表」，数组形式则每个 ASN 各建一个 ASxxxx 条目。
	wantList := make(map[string][]string)
	for list, asnList := range tmp.Want.TypeMap {
		list = strings.ToUpper(strings.TrimSpace(list))
		if list == "" {
			continue
		}
		for _, asn := range asnList {
			if asn = lib.NormalizeASN(asn); asn == "" {
				continue
			}
			wantList[asn] = append(wantList[asn], list)
		}
	}
	for _, asn := range tmp.Want.TypeSlice {
		if asn = lib.NormalizeASN(asn); asn == "" {
			continue
		}
		wantList[asn] = append(wantList[asn], "AS"+asn)
	}

	// 这里刻意不允许「不指定 wantedList」：iptoasn 全集有 10 万个以上的 ASN，
	// 全部落成条目会把 Country.mmdb 撑到不可用的体积。
	// 上游同语义的插件允许留空，但在本项目的产物形态下那是个陷阱，故直接拒绝。
	if len(wantList) == 0 {
		return nil, fmt.Errorf("❌ [type %s | action %s] wantedList must be specified with the ASNs you want to group", TypeIPToASN, action)
	}

	return &IPToASNIn{
		Type:        TypeIPToASN,
		Action:      action,
		Description: DescIPToASN,
		IPv4File:    tmp.IPv4File,
		IPv6File:    tmp.IPv6File,
		Want:        wantList,
		OnlyIPType:  tmp.OnlyIPType,
	}, nil
}

type IPToASNIn struct {
	Type        string
	Action      lib.Action
	Description string
	IPv4File    string
	IPv6File    string
	Want        map[string][]string
	OnlyIPType  lib.IPType
}

func (g *IPToASNIn) GetType() string {
	return g.Type
}

func (g *IPToASNIn) GetAction() lib.Action {
	return g.Action
}

func (g *IPToASNIn) GetDescription() string {
	return g.Description
}

func (g *IPToASNIn) Input(container lib.Container) (lib.Container, error) {
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

func (g *IPToASNIn) process(file string, entries map[string]*lib.Entry) error {
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

		// AS 号为 0 表示「已分配但未路由」，iptoasn 用它占位，不是真实 ASN
		asn := strings.TrimSpace(fields[2])
		if asn == "" || asn == "0" {
			continue
		}

		listNames, wanted := g.Want[asn]
		if !wanted {
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

		for _, listName := range listNames {
			if err := lib.AddIPRangeToEntries(entries, listName, ipRange); err != nil {
				return err
			}
		}
		matched++
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if matched == 0 {
		return fmt.Errorf("❌ [type %s | action %s] %s matched no configured ASN, please check wantedList", g.Type, g.Action, file)
	}

	return nil
}
