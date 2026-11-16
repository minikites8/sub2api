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
	require.Len(t, spec.CipherSuites, 13)
	require.Len(t, spec.Extensions, 11)
	require.Equal(t, uint16(utls.VersionTLS12), spec.TLSVersMin)
	for _, extension := range spec.Extensions {
		switch extension.(type) {
		case *utls.ALPNExtension, *utls.UtlsGREASEExtension, *utls.SessionTicketExtension, *utls.PSKKeyExchangeModesExtension:
			t.Fatalf("unexpected extension %T in Bun capture", extension)
		}
	}
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	connection := utls.UClient(client, &utls.Config{ServerName: "opencode.ai"}, utls.HelloCustom)
	require.NoError(t, connection.ApplyPreset(spec))
	require.NoError(t, connection.BuildHandshakeState())
	// Factory calls must not share mutable backing arrays.
	profile.CipherSuites[0] = 0
	require.NotZero(t, OpenCodeBunProfile().CipherSuites[0])
}
