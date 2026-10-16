package v2ray

import (
	"testing"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestSchemaRoundTrip(t *testing.T) {
	options := File_geoip_proto.Options().(*descriptorpb.FileOptions)
	if options.GetGoPackage() != "github.com/laincat/GeoIP/plugin/v2ray" { t.Fatal(options.GetGoPackage()) }
	original := &GeoIPList{Entry: []*GeoIP{{CountryCode: "US", Cidr: []*CIDR{{Ip: []byte{8, 8, 8, 0}, Prefix: 24}}}}}
	data, err := proto.Marshal(original)
	if err != nil { t.Fatal(err) }
	decoded := new(GeoIPList)
	if err := proto.Unmarshal(data, decoded); err != nil { t.Fatal(err) }
	if !proto.Equal(original, decoded) { t.Fatal("wire format changed") }
}
