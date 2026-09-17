package plaintext

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/laincat/GeoIP/lib"
)

const (
	TypeTextIn = "text"
	DescTextIn = "Convert plaintext IP & CIDR to other formats"
)

func init() {
	lib.RegisterInputConfigCreator(TypeTextIn, func(action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
		return newTextIn(TypeTextIn, DescTextIn, action, data)
	})
	lib.RegisterInputConverter(TypeTextIn, &TextIn{
		Description: DescTextIn,
	})
}

func newTextIn(iType string, iDesc string, action lib.Action, data json.RawMessage) (lib.InputConverter, error) {
	var tmp struct {
		Name       string     `json:"name"`
		URI        string     `json:"uri"`
		IPOrCIDR   []string   `json:"ipOrCIDR"`
		InputDir   string     `json:"inputDir"`
		Want       []string   `json:"wantedList"`
		OnlyIPType lib.IPType `json:"onlyIPType"`

		JSONPath             []string `json:"jsonPath"`
		RemovePrefixesInLine []string `json:"removePrefixesInLine"`
		RemoveSuffixesInLine []string `json:"removeSuffixesInLine"`
	}

	if strings.TrimSpace(iType) == "" {
		return nil, fmt.Errorf("type is required")
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
	}

	if iType != TypeTextIn && len(tmp.IPOrCIDR) > 0 {
		return nil, fmt.Errorf("❌ [type %s | action %s] ipOrCIDR is invalid for this input format", iType, action)
	}

	if iType == TypeJSONIn && len(tmp.JSONPath) == 0 {
		return nil, fmt.Errorf("❌ [type %s | action %s] missing jsonPath", iType, action)
	}

	if tmp.InputDir == "" {
		if tmp.Name == "" {
			return nil, fmt.Errorf("❌ [type %s | action %s] missing inputDir or name", iType, action)
		}
		if tmp.URI == "" && len(tmp.IPOrCIDR) == 0 {
			return nil, fmt.Errorf("❌ [type %s | action %s] missing uri or ipOrCIDR", iType, action)
		}
	} else if tmp.Name != "" || tmp.URI != "" || len(tmp.IPOrCIDR) > 0 {
		return nil, fmt.Errorf("❌ [type %s | action %s] inputDir is not allowed to be used with name or uri or ipOrCIDR", iType, action)
	}

	// Filter want list
	wantList := make(map[string]bool)
	for _, want := range tmp.Want {
		if want = strings.ToUpper(strings.TrimSpace(want)); want != "" {
			wantList[want] = true
		}
	}

	return &TextIn{
		Type:        iType,
		Action:      action,
		Description: iDesc,
		Name:        tmp.Name,
		URI:         tmp.URI,
		IPOrCIDR:    tmp.IPOrCIDR,
		InputDir:    tmp.InputDir,
		Want:        wantList,
		OnlyIPType:  tmp.OnlyIPType,

		JSONPath:             tmp.JSONPath,
		RemovePrefixesInLine: tmp.RemovePrefixesInLine,
		RemoveSuffixesInLine: tmp.RemoveSuffixesInLine,
	}, nil
}

func (t *TextIn) GetType() string {
	return t.Type
}

func (t *TextIn) GetAction() lib.Action {
	return t.Action
}

func (t *TextIn) GetDescription() string {
	return t.Description
}

func (t *TextIn) Input(container lib.Container) (lib.Container, error) {
	entries := make(map[string]*lib.Entry)
	var err error

	switch {
	case t.InputDir != "":
		err = t.walkDir(t.InputDir, entries)

	case t.Name != "" && t.URI != "":
		if lib.IsRemoteURI(t.URI) {
			err = t.walkRemoteFile(t.URI, t.Name, entries)
		} else {
			err = t.walkLocalFile(t.URI, t.Name, entries)
		}
		if err != nil {
			return nil, err
		}

		fallthrough

	case t.Name != "" && len(t.IPOrCIDR) > 0:
		err = t.appendIPOrCIDR(t.IPOrCIDR, t.Name, entries)

	default:
		return nil, fmt.Errorf("❌ [type %s | action %s] config missing argument inputDir or name or uri or ipOrCIDR", t.Type, t.Action)
	}

	if err != nil {
		return nil, err
	}

	ignoreIPType := lib.GetIgnoreIPType(t.OnlyIPType)

	if len(entries) == 0 {
		return nil, fmt.Errorf("❌ [type %s | action %s] no entry is generated", t.Type, t.Action)
	}

	for _, entry := range entries {
		switch t.Action {
		case lib.ActionAdd:
			if err := container.Add(entry, ignoreIPType); err != nil {
				return nil, err
			}
		case lib.ActionRemove:
			if err := container.Remove(entry, lib.CaseRemovePrefix, ignoreIPType); err != nil {
				return nil, err
			}
		default:
			return nil, lib.ErrUnknownAction
		}
	}

	return container, nil
}

func (t *TextIn) walkDir(dir string, entries map[string]*lib.Entry) error {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		if err := t.walkLocalFile(path, "", entries); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (t *TextIn) walkLocalFile(path, name string, entries map[string]*lib.Entry) error {
	entryName := ""
	name = strings.TrimSpace(name)
	if name != "" {
		entryName = name
	} else {
		entryName = filepath.Base(path)

		// check filename
		if !regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`).MatchString(entryName) {
			return fmt.Errorf("❌ [type %s | action %s] filename %s cannot be entry name, please remove special characters in it", t.Type, t.Action, entryName)
		}

		// remove file extension but not hidden files of which filename starts with "."
		dotIndex := strings.LastIndex(entryName, ".")
		if dotIndex > 0 {
			entryName = entryName[:dotIndex]
		}
	}

	entryName = strings.ToUpper(entryName)

	if len(t.Want) > 0 && !t.Want[entryName] {
		return nil
	}
	if _, found := entries[entryName]; found {
		return fmt.Errorf("❌ [type %s | action %s] found duplicated list %s", t.Type, t.Action, entryName)
	}

	entry := lib.NewEntry(entryName)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := t.scanFile(file, entry); err != nil {
		return err
	}

	entries[entryName] = entry

	return nil
}

func (t *TextIn) walkRemoteFile(url, name string, entries map[string]*lib.Entry) error {
	// 走 lib.GetRemoteURLReader，不自己 http.Get —— 超时、重试与 User-Agent
	// 统一由 lib 那一层负责。这里原来自己写了一遍，结果是「加超时」时漏掉了它：
	// http.Get 用的是零值 client，没有超时，源站卡住就会把 CI 挂到 6 小时上限。
	body, err := lib.GetRemoteURLReader(url)
	if err != nil {
		return fmt.Errorf("❌ [type %s | action %s] %w", t.Type, t.Action, err)
	}
	defer body.Close()

	name = strings.ToUpper(name)

	if len(t.Want) > 0 && !t.Want[name] {
		return nil
	}

	entry := lib.NewEntry(name)
	if err := t.scanFile(body, entry); err != nil {
		return err
	}

	entries[name] = entry

	return nil
}

func (t *TextIn) appendIPOrCIDR(ipOrCIDR []string, name string, entries map[string]*lib.Entry) error {
	name = strings.ToUpper(name)

	entry, found := entries[name]
	if !found {
		entry = lib.NewEntry(name)
	}

	for _, cidr := range ipOrCIDR {
		if err := entry.AddPrefix(strings.TrimSpace(cidr)); err != nil {
			return err
		}
	}

	entries[name] = entry

	return nil
}
