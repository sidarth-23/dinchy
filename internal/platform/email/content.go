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

func invitationContent(data InvitationEmail, ctaURL string) presentation {
	organisation := i18n.P("organisation", data.OrganisationName)
	return presentation{
		Subject:  resolve(i18n.Msg(i18n.CodeEmailInvitationSubject, organisation)),
		Heading:  resolve(i18n.Msg(i18n.CodeEmailInvitationHeading, organisation)),
		Body:     resolve(i18n.Msg(i18n.CodeEmailInvitationBody, organisation, i18n.P("role", data.Role))),
		CTALabel: resolve(i18n.Msg(i18n.CodeEmailInvitationCta)),
		CTAURL:   ctaURL,
		Footer:   resolve(i18n.Msg(i18n.CodeEmailFooter)),
	}
}

func passwordResetContent(ctaURL string) presentation {
	return presentation{
		Subject:  resolve(i18n.Msg(i18n.CodeEmailPasswordResetSubject)),
		Heading:  resolve(i18n.Msg(i18n.CodeEmailPasswordResetHeading)),
		Body:     resolve(i18n.Msg(i18n.CodeEmailPasswordResetBody)),
		CTALabel: resolve(i18n.Msg(i18n.CodeEmailPasswordResetCta)),
		CTAURL:   ctaURL,
		Footer:   resolve(i18n.Msg(i18n.CodeEmailFooter)),
	}
}
