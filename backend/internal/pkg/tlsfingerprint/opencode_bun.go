package tlsfingerprint

// OpenCodeBunProfileName distinguishes the built-in Bun capture from user
// profiles, including its renegotiation and TLS-version behavior.
const OpenCodeBunProfileName = "OpenCode 2.2.0 Bun 1.4.4"

// OpenCodeBunProfile describes a fresh TLS connection captured from OpenCode
// 2.2.0's embedded Bun 1.4.4 fetch on 2026-11-17. The older OpenCode 1.19.x
// capture returns FreeUsageLimitError even with otherwise valid requests.
// A fresh profile is returned so callers cannot mutate shared slice storage.
func OpenCodeBunProfile() *Profile {
	return &Profile{
		Name: OpenCodeBunProfileName,
		CipherSuites: []uint16{
			0x1301, 0x1302, 0x1303,
			0xc02b, 0xc02f, 0xc02c, 0xc030, 0xcca9, 0xcca8,
			0xc009, 0xc013, 0xc00a, 0xc014, 0x009c, 0x009d, 0x002f, 0x0035,
		},
		Curves:       []uint16{0x11ec, 0x001d, 0x0017, 0x0018},
		PointFormats: []uint16{0},
		SignatureAlgorithms: []uint16{
			0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501,
			0x0806, 0x0601, 0x0807, 0x0603, 0x0201,
		},
		ALPNProtocols:     []string{"http/1.1"},
		SupportedVersions: []uint16{0x0304, 0x0303},
		KeyShareGroups:    []uint16{0x11ec, 0x001d},
		Extensions:        []uint16{0, 23, 65281, 10, 11, 35, 16, 5, 13, 18, 51, 45, 43},
	}
}
