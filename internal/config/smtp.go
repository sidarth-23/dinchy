package config

// SMTPConfig holds the outbound email settings for password reset and invite flows.
type SMTPConfig struct {
	// Host is the SMTP server hostname used for outbound application email.
	Host string `env:"DINCHY_SMTP_HOST" mod:"trim"`
	// Port is the SMTP server port; defaults to 587 when SMTP is enabled and no port is set.
	Port uint16 `env:"DINCHY_SMTP_PORT" validate:"gt=0,lte=65535"`
	// Username is the optional SMTP username for authenticated mail servers.
	Username string `env:"DINCHY_SMTP_USERNAME"`
	// Password is the optional SMTP password for authenticated mail servers.
	Password string `env:"DINCHY_SMTP_PASSWORD"`
	// From is the sender address used for password reset and invite emails.
	From string `env:"DINCHY_SMTP_FROM" mod:"trim"`
	// TLS selects the STARTTLS policy for outbound mail: "mandatory" (default,
	// require STARTTLS), "opportunistic" (use STARTTLS when the server offers it),
	// or "off" (plain connection, for local catchers such as Mailpit).
	TLS string `env:"DINCHY_SMTP_TLS" mod:"trim,lower" validate:"omitempty,oneof=mandatory opportunistic off"`
}

// DefaultSMTP returns the default SMTP configuration used when no
// environment overrides are provided.
func DefaultSMTP() SMTPConfig {
	return SMTPConfig{Port: 587}
}

// Enabled reports whether outbound email is configured.
func (c SMTPConfig) Enabled() bool {
	return c.Host != "" || c.From != ""
}
