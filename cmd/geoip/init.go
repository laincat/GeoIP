package main

import (
	// 数据源插件：IPInfo（国家维度）与 iptoasn.com（ASN 维度）
	_ "github.com/laincat/GeoIP/plugin/ipinfo"
	_ "github.com/laincat/GeoIP/plugin/iptoasn"

	// 输出插件：MaxMind mmdb（含 Country 与 GeoLite2-ASN 兼容产物）
	_ "github.com/laincat/GeoIP/plugin/maxmind"

	// 输出插件：V2Ray GeoIP dat
	_ "github.com/laincat/GeoIP/plugin/v2ray"

	// 通用输入：plaintext（text、json、surge、clash）与 special（private、cutter 等）
	_ "github.com/laincat/GeoIP/plugin/plaintext"
	_ "github.com/laincat/GeoIP/plugin/special"
)
