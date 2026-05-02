package i18n

import "golang.org/x/text/language"

// Default is the package-level catalog pre-loaded with English translations.
// Additional locales can be registered before the server starts.
var Default = func() *Catalog {
	c := New(language.English)
	c.Register(language.English, En)
	return c
}()
