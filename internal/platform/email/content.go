package email

import (
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// presentation is the fully-resolved, localized copy handed to the layout templates.
type presentation struct {
	Subject  string
	Heading  string
	Body     string
	CTALabel string
	CTAURL   string
	Footer   string
}

// resolve renders a catalog message at the default locale. Recipient locale is
// unknown for invitees, so email copy always uses the catalog's default language.
func resolve(msg i18n.Message) string {
	return i18n.Default.Resolve(i18n.Default.Match(""), msg)
}
