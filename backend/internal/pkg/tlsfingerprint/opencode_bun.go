package tlsfingerprint

// OpenCodeBunProfileName distinguishes the built-in Bun capture from user
// profiles, including its renegotiation and TLS-version behavior.
const OpenCodeBunProfileName = "OpenCode Bun (1.19.x capture)"

// OpenCodeBunProfile describes the Bun/BoringSSL ClientHello captured by
// azain47/zen-proxy. In particular, its extension list omits ALPN, tickets and
// GREASE; using the default Node.js profile changes this fingerprint.
// A fresh profile is returned so callers cannot mutate shared slice storage.
func OpenCodeBunProfile() *Profile {
	return &Profile{
		Name: OpenCodeBunProfileName,
		CipherSuites: []uint16{
			0xc02b, 0xc02f, 0xc02c, 0xc030, 0xcca9, 0xcca8,
			0xc009, 0xc013, 0xc00a, 0xc014, 0x1301, 0x1302, 0x1303,
		},
		Curves:       []uint16{0x11ec, 0x001d, 0x0017, 0x0018, 0x0019},
		PointFormats: []uint16{0},
		SignatureAlgorithms: []uint16{
			0x0904, 0x0905, 0x0906, 0x0804, 0x0403, 0x0807,
			0x0805, 0x0806, 0x0401, 0x0501, 0x0601, 0x0503,
			0x0603, 0x0201, 0x0203,
		},
		SupportedVersions: []uint16{0x0304, 0x0303},
		KeyShareGroups:    []uint16{0x11ec, 0x001d},
		Extensions:        []uint16{0, 11, 65281, 23, 18, 5, 10, 13, 50, 43, 51},
	}
}
