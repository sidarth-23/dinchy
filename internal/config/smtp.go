package config

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
}

func DefaultSMTP() SMTPConfig {
	return SMTPConfig{Port: 587}
}

func (c SMTPConfig) Enabled() bool {
	return c.Host != "" || c.From != ""
}
