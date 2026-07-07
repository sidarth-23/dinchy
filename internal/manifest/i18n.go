package manifest

import (
	"fmt"
	"strings"
)

// I18nCatalog is the root of the localization manifest.
type I18nCatalog struct {
	Modules []I18nModule `json:"modules"`
}

// I18nModule is a namespace grouping localized messages and nested modules.
type I18nModule struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Modules     []I18nModule  `json:"modules,omitempty"`
	Messages    []I18nMessage `json:"messages,omitempty"`
}

// I18nMessage is a translatable message with its per-language text and params.
type I18nMessage struct {
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Params       []I18nParam       `json:"params,omitempty"`
	Translations map[string]string `json:"translations"`
}

// I18nParam is a named, typed substitution slot within a message.
type I18nParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// DecodeI18nCatalog strictly decodes raw JSON into an I18nCatalog.
func DecodeI18nCatalog(raw []byte) (I18nCatalog, error) {
	var catalog I18nCatalog
	if err := decodeStrict(raw, &catalog); err != nil {
		return I18nCatalog{}, err
	}
	return catalog, nil
}

// ValidateI18nCatalog reports whether the catalog is well-formed, has no
// duplicate codes, constant names, or params, and includes an en translation.
func ValidateI18nCatalog(catalog I18nCatalog) error {
	if len(catalog.Modules) == 0 {
		return fmt.Errorf("i18n catalog must define at least one module")
	}

	seenCodes := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	langs := map[string]struct{}{}
	return validateI18nModules(catalog.Modules, nil, seenCodes, seenNames, langs)
}

func validateI18nModules(modules []I18nModule, modulePath []string, seenCodes, seenNames, langs map[string]struct{}) error {
	seenModuleNames := map[string]struct{}{}
	for _, module := range modules {
		if module.Name == "" {
			return fmt.Errorf("module name cannot be empty")
		}
		if _, ok := seenModuleNames[module.Name]; ok {
			return fmt.Errorf("duplicate module name %q within module path %q", module.Name, DisplayPath(modulePath))
		}
		seenModuleNames[module.Name] = struct{}{}

		currentPath := append(append([]string{}, modulePath...), module.Name)
		if len(module.Messages) == 0 && len(module.Modules) == 0 {
			return fmt.Errorf("module %q must define at least one child message or module", DisplayPath(currentPath))
		}

		seenMessageNames := map[string]struct{}{}
		for _, message := range module.Messages {
			if message.Name == "" {
				return fmt.Errorf("message name cannot be empty within module path %q", DisplayPath(currentPath))
			}
			if _, ok := seenMessageNames[message.Name]; ok {
				return fmt.Errorf("duplicate message name %q within module path %q", message.Name, DisplayPath(currentPath))
			}
			seenMessageNames[message.Name] = struct{}{}

			code := I18nCodeFor(currentPath, message.Name)
			constName := I18nConstantName(currentPath, message.Name)
			if _, ok := seenCodes[code]; ok {
				return fmt.Errorf("duplicate message code %q", code)
			}
			if _, ok := seenNames[constName]; ok {
				return fmt.Errorf("duplicate generated constant name %q", constName)
			}
			seenCodes[code] = struct{}{}
			seenNames[constName] = struct{}{}

			if len(message.Translations) == 0 {
				return fmt.Errorf("message %q has no translations", DisplayPath(append(currentPath, message.Name)))
			}

			for lang, text := range message.Translations {
				if lang == "" {
					return fmt.Errorf("message %q has an empty translation language", DisplayPath(append(currentPath, message.Name)))
				}
				if text == "" {
					return fmt.Errorf("message %q has empty translation for %q", DisplayPath(append(currentPath, message.Name)), lang)
				}
				langs[lang] = struct{}{}
			}

			paramNames := map[string]struct{}{}
			for _, param := range message.Params {
				if param.Name == "" {
					return fmt.Errorf("message %q has a param with empty name", DisplayPath(append(currentPath, message.Name)))
				}
				if param.Type == "" {
					return fmt.Errorf("message %q param %q has empty type", DisplayPath(append(currentPath, message.Name)), param.Name)
				}
				if _, ok := paramNames[param.Name]; ok {
					return fmt.Errorf("message %q has duplicate param %q", DisplayPath(append(currentPath, message.Name)), param.Name)
				}
				paramNames[param.Name] = struct{}{}
			}
		}

		if err := validateI18nModules(module.Modules, currentPath, seenCodes, seenNames, langs); err != nil {
			return err
		}
	}

	if len(modulePath) == 0 {
		if _, ok := langs["en"]; !ok {
			return fmt.Errorf("manifest must include an en translation")
		}
	}
	return nil
}

// I18nCodeFor returns the dot-joined message code for a module path and name.
func I18nCodeFor(modulePath []string, messageName string) string {
	parts := append(append([]string{}, modulePath...), messageName)
	return strings.Join(parts, ".")
}

// I18nConstantName returns the generated Go constant name for a message.
func I18nConstantName(modulePath []string, messageName string) string {
	parts := append(append([]string{}, modulePath...), messageName)
	return GoNameFromPath(parts...)
}
