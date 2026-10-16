package ipinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/laincat/GeoIP/lib"
)

func TestCountryCSVHeadersAndInvalidRows(t *testing.T) {
	for _, tc := range []struct { data string; valid bool } {
		{"country,end_ip,start_ip\nUS,8.8.8.255,8.8.8.0\n", true},
		{"start_ip,end_ip,country\n2001:db8::,2001:db8::ffff,US\n", true},
		{"8.8.8.0,8.8.8.255,US\n", true},
		{"start_ip,end_ip\n8.8.8.0,8.8.8.255\n", false},
		{"start_ip,end_ip,country\n8.8.8.0,8.8.8.255,US\nbad,row,FR\n", false},
		{"start_ip,end_ip,country\n8.8.8.255,8.8.8.0,US\n", false},
	} {
		file := filepath.Join(t.TempDir(), "country.csv")
		if err := os.WriteFile(file, []byte(tc.data), 0600); err != nil { t.Fatal(err) }
		input := &CountryCSVIn{URI: file, Action: lib.ActionAdd}
		_, err := input.Input(lib.NewContainer())
		if (err == nil) != tc.valid { t.Fatalf("valid=%v, err=%v for %s", tc.valid, err, tc.data) }
	}
}
