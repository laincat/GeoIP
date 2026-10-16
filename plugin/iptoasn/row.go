package iptoasn

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/laincat/GeoIP/lib"
	"go4.org/netipx"
)

// Validate every source row before filtering so malformed records cannot silently
// erase coverage. ASN zero is a valid unassigned range, not malformed input.
func parseRow(line string) (netipx.IPRange, string, string, error) {
	fields := strings.SplitN(line, "\t", 5)
	if len(fields) < 5 {
		return netipx.IPRange{}, "", "", fmt.Errorf("ASN row requires five columns")
	}
	asn, err := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 32)
	if err != nil {
		return netipx.IPRange{}, "", "", fmt.Errorf("invalid ASN: %w", err)
	}
	start, ok := lib.ParseAddrString(fields[0])
	if !ok {
		return netipx.IPRange{}, "", "", fmt.Errorf("invalid ASN start address")
	}
	end, ok := lib.ParseAddrString(fields[1])
	if !ok {
		return netipx.IPRange{}, "", "", fmt.Errorf("invalid ASN end address")
	}
	r := netipx.IPRangeFrom(start, end)
	if !r.IsValid() {
		return netipx.IPRange{}, "", "", fmt.Errorf("invalid ASN address range")
	}
	return r, strconv.FormatUint(asn, 10), strings.TrimSpace(fields[4]), nil
}
