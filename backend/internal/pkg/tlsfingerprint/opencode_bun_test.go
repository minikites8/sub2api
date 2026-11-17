package tlsfingerprint

import (
	"net"
	"testing"

	utls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeBunClientHello(t *testing.T) {
	profile := OpenCodeBunProfile()
	spec := buildClientHelloSpecFromProfile(profile)
	require.Equal(t, []uint16{
		0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f, 0xc02c, 0xc030,
		0xcca9, 0xcca8, 0xc009, 0xc013, 0xc00a, 0xc014, 0x009c, 0x009d, 0x002f, 0x0035,
	}, spec.CipherSuites, "fresh OpenCode 2.2.0/Bun 1.4.4 capture")
	require.Len(t, spec.Extensions, 13)
	require.Equal(t, uint16(utls.VersionTLS12), spec.TLSVersMin)
	var extensionIDs []uint16
	for _, extension := range spec.Extensions {
		if sni, ok := extension.(*utls.SNIExtension); ok {
			sni.ServerName = "opencode.ai"
		}
		data := make([]byte, extension.Len())
		require.GreaterOrEqual(t, len(data), 4, "extension %T", extension)
		_, _ = extension.Read(data)
		extensionIDs = append(extensionIDs, uint16(data[0])<<8|uint16(data[1]))
		switch extension.(type) {
		case *utls.UtlsGREASEExtension, *utls.SignatureAlgorithmsCertExtension:
			t.Fatalf("unexpected extension %T in Bun capture", extension)
		}
	}
	require.Equal(t, []uint16{0, 23, 65281, 10, 11, 35, 16, 5, 13, 18, 51, 45, 43}, extensionIDs)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	connection := utls.UClient(client, &utls.Config{ServerName: "opencode.ai"}, utls.HelloCustom)
	require.NoError(t, connection.ApplyPreset(spec))
	require.NoError(t, connection.BuildHandshakeState())
	hello := connection.HandshakeState.Hello
	require.Equal(t, []utls.CurveID{0x11ec, 0x001d, 0x0017, 0x0018}, hello.SupportedCurves)
	require.Equal(t, []string{"http/1.1"}, hello.AlpnProtocols)
	require.Len(t, hello.KeyShares, 2)
	// Factory calls must not share mutable backing arrays.
	profile.CipherSuites[0] = 0
	require.NotZero(t, OpenCodeBunProfile().CipherSuites[0])
}
