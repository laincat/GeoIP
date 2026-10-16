package iptoasn

import "testing"

func TestParseRow(t *testing.T) {
	for _, line := range []string{
		"1.1.1.0\t1.1.1.255\t13335\tUS\tCloudflare",
		"2001:db8::\t2001:db8::ffff\t42\tUS\tIPv6 organization",
		"1.1.1.0\t1.1.1.255\t0\tNone\tNot routed",
	} {
		r, _, org, err := parseRow(line)
		if err != nil || !r.IsValid() || org == "" { t.Fatalf("%s: %v", line, err) }
	}
	for _, line := range []string{
		"bad", "1.1.1.0\t1.1.1.255\tINVALID\tUS\tOrg",
		"1.1.1.255\t1.1.1.0\t42\tUS\tOrg",
		"1.1.1.0\t::1\t42\tUS\tOrg",
		"1.1.1.0\t1.1.1.255\t4294967296\tUS\tOrg",
	} {
		if _, _, _, err := parseRow(line); err == nil { t.Fatalf("accepted %s", line) }
	}
}
