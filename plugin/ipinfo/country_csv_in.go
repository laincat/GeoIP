package ipinfo

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"path/filepath"
	"slices"
	"strings"

	"github.com/laincat/GeoIP/lib"
	"go4.org/netipx"
)

const (
	TypeCountryCSV = "ipinfoCountryCSV"
	DescCountryCSV = "Convert IPInfo free country CSV data to other formats"
)

var defaultCountryCSVFile = filepath.Join("./", "ipinfo", "country.csv")

// IPInfo 免费数据集 country.csv 与 country_asn.csv 都是「起止 IP + 国家」的区间表，
// 后者额外多出 asn / as_name / as_domain 三列。因为两者列序一致、只是长度不同，
// 这里用同一个转换器处理：列多就把 ASN 也用上，列少就只出国家。
//
// 参考：https://ipinfo.io/data
func init() {
	lib.RegisterInputConfigCreator(TypeCountryCSV, func(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
		return newCountryCSVIn(action, data)
	})
	lib.RegisterInputConverter(TypeCountryCSV, &CountryCSVIn{
		Description: DescCountryCSV,
	})
}

func newCountryCSVIn(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
	var tmp struct {
		URI        string                 `json:"uri"`
		Want       []string               `json:"wantedList"`
		ASNList    lib.WantedListExtended `json:"asnList"`
		OnlyIPType lib.IPType             `json:"onlyIPType"`
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(tmp.URI) == "" {
		tmp.URI = defaultCountryCSVFile
	}

	// 国家白名单
	wantList := make(map[string]bool)
	for _, want := range tmp.Want {
		if want = strings.ToUpper(strings.TrimSpace(want)); want != "" {
			wantList[want] = true
		}
	}

	// ASN -> 类别名，语义与上游 maxmindGeoLite2ASNCSV 保持一致：
	// map 形式是「类别 -> ASN 列表」（多个 ASN 归并进同一类别），
	// 数组形式则是每个 ASN 各自建一个 ASxxxx 条目。
	wantASN := make(map[string][]string)
	for list, asnList := range tmp.ASNList.TypeMap {
		list = strings.ToUpper(strings.TrimSpace(list))
		if list == "" {
			continue
		}
		for _, asn := range asnList {
			if asn = lib.NormalizeASN(asn); asn == "" {
				continue
			}
			wantASN[asn] = append(wantASN[asn], list)
		}
	}
	for _, asn := range tmp.ASNList.TypeSlice {
		if asn = lib.NormalizeASN(asn); asn == "" {
			continue
		}
		wantASN[asn] = append(wantASN[asn], "AS"+asn)
	}

	return &CountryCSVIn{
		Type:        TypeCountryCSV,
		Action:      action,
		Description: DescCountryCSV,
		URI:         tmp.URI,
		Want:        wantList,
		WantASN:     wantASN,
		OnlyIPType:  tmp.OnlyIPType,
	}, nil
}

type CountryCSVIn struct {
	Type        string
	Action      lib.Action
	Description string
	URI         string
	Want        map[string]bool
	WantASN     map[string][]string
	OnlyIPType  lib.IPType
}

func (c *CountryCSVIn) GetType() string {
	return c.Type
}

func (c *CountryCSVIn) GetAction() lib.Action {
	return c.Action
}

func (c *CountryCSVIn) GetDescription() string {
	return c.Description
}

// columnIndex 记录关心的几列各自的下标，-1 表示该列不存在。
type columnIndex struct {
	start   int
	end     int
	country int
	asn     int
}

func (c *CountryCSVIn) Input(container lib.Container) (lib.Container, error) {
	rc, err := lib.OpenMaybeGzip(c.URI)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	reader := csv.NewReader(rc)
	reader.FieldsPerRecord = -1 // IPInfo 不同数据集列数不同，不做齐整校验
	reader.ReuseRecord = true

	cols, pending, err := c.resolveColumns(reader)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]*lib.Entry)
	rows, skipped := 0, 0

	for {
		record := pending
		pending = nil

		if record == nil {
			record, err = reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("❌ [type %s | action %s] failed to read CSV: %w", c.Type, c.Action, err)
			}
		}

		rows++

		start, ok := parseAddr(record, cols.start)
		if !ok {
			skipped++
			continue
		}
		end, ok := parseAddr(record, cols.end)
		if !ok {
			skipped++
			continue
		}

		ipRange := netipx.IPRangeFrom(start, end)
		if !ipRange.IsValid() {
			skipped++
			continue
		}
		if lib.SkipByIPType(ipRange, c.OnlyIPType) {
			continue
		}

		if code, ok := fieldAt(record, cols.country); ok {
			if code = strings.ToUpper(code); code != "" && (len(c.Want) == 0 || c.Want[code]) {
				if err := lib.AddIPRangeToEntries(entries, code, ipRange); err != nil {
					return nil, err
				}
			}
		}

		if len(c.WantASN) > 0 {
			if asn, ok := fieldAt(record, cols.asn); ok {
				for _, listName := range c.WantASN[asn] {
					if err := lib.AddIPRangeToEntries(entries, listName, ipRange); err != nil {
						return nil, err
					}
				}
			}
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf(
			"❌ [type %s | action %s] no entry is generated from %s (read %d data rows, skipped %d)",
			c.Type, c.Action, c.URI, rows, skipped)
	}

	return lib.ApplyEntries(container, entries, c.Action, c.OnlyIPType)
}

// resolveColumns 判定首行是表头还是数据，并给出各列的落点。
//
// 之所以先认表头名而不是写死 0/1/2/6：IPInfo 的数据集在扩容时确实加过列
// （country -> country_asn 就是这么来的），按名字定位能在它下次插列时继续工作；
// 只有当表头完全不符合预期（例如无表头的裸转储）时，才回落到官方当前列序。
func (c *CountryCSVIn) resolveColumns(reader *csv.Reader) (columnIndex, []string, error) {
	first, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return columnIndex{}, nil, fmt.Errorf("❌ [type %s | action %s] %s is empty", c.Type, c.Action, c.URI)
		}
		return columnIndex{}, nil, fmt.Errorf("❌ [type %s | action %s] failed to read CSV: %w", c.Type, c.Action, err)
	}

	// reader.ReuseRecord 为 true 时底层切片会被后续 Read 复用，所以必须留副本
	first = slices.Clone(first)

	cols := columnIndex{start: -1, end: -1, country: -1, asn: -1}
	for i, cell := range first {
		switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(cell, "\ufeff"))) {
		case "start_ip":
			cols.start = i
		case "end_ip":
			cols.end = i
		case "country":
			cols.country = i
		case "asn":
			cols.asn = i
		}
	}

	if cols.start >= 0 && cols.end >= 0 {
		return cols, nil, nil // 首行是表头，已消费
	}

	// 首行不是表头，那它就是数据：按官方列序兜底，并把它交回主循环
	cols = columnIndex{start: 0, end: 1, country: 2, asn: 6}
	return cols, first, nil
}

func fieldAt(record []string, idx int) (string, bool) {
	if idx < 0 || idx >= len(record) {
		return "", false
	}
	return strings.TrimSpace(record[idx]), true
}

func parseAddr(record []string, idx int) (netip.Addr, bool) {
	raw, ok := fieldAt(record, idx)
	if !ok {
		return netip.Addr{}, false
	}
	return lib.ParseAddrString(raw)
}
