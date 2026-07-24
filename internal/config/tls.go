package config

// TLSConfig holds the certificate and key for the public HTTPS listener. When both
// are set the server terminates TLS directly; when neither is set it serves plain
// HTTP, for a deployment that terminates TLS at an external proxy.
type TLSConfig struct {
	// CertFile is the path to the PEM-encoded TLS certificate (with chain).
	CertFile string `env:"DINCHY_TLS_CERT_FILE" mod:"trim"`
	// KeyFile is the path to the PEM-encoded TLS private key.
	KeyFile string `env:"DINCHY_TLS_KEY_FILE" mod:"trim"`
}

// Enabled reports whether the server should terminate TLS directly.
func (c TLSConfig) Enabled() bool {
	return c.CertFile != "" && c.KeyFile != ""
}
