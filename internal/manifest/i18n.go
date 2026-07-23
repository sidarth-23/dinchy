package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// LoadI18nCatalog loads a catalog from path, which may be either a directory of
// *.json fragments or a single fragment file. It decodes and merges but does not
// validate; callers validate the result.
func LoadI18nCatalog(path string) (I18nCatalog, error) {
	info, err := os.Stat(path)
	if err != nil {
		return I18nCatalog{}, fmt.Errorf("stat i18n catalog %q: %w", path, err)
	}
	if info.IsDir() {
		return LoadI18nCatalogDir(path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return I18nCatalog{}, fmt.Errorf("read i18n catalog %q: %w", path, err)
	}
	return DecodeI18nCatalog(raw)
}

// LoadI18nCatalogDir reads every *.json fragment in dir (in sorted filename
// order), strictly decodes each, and deep-merges them into one catalog. It does
// not validate the result; callers validate the merged catalog. It errors if dir
// contains no fragments so an empty directory fails loudly at its source.
func LoadI18nCatalogDir(dir string) (I18nCatalog, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return I18nCatalog{}, fmt.Errorf("scan i18n catalog directory %q: %w", dir, err)
	}
	if len(matches) == 0 {
		return I18nCatalog{}, fmt.Errorf("no i18n fragments found in %q", dir)
	}
	sort.Strings(matches)

	catalogs := make([]I18nCatalog, 0, len(matches))
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			return I18nCatalog{}, fmt.Errorf("read i18n fragment %q: %w", path, err)
		}
		catalog, err := DecodeI18nCatalog(raw)
		if err != nil {
			return I18nCatalog{}, fmt.Errorf("decode i18n fragment %q: %w", path, err)
		}
		catalogs = append(catalogs, catalog)
	}
	return MergeI18nCatalogs(catalogs...)
}

// MergeI18nCatalogs deep-merges catalogs by module name at each level. Modules
// sharing a name under the same parent are merged recursively; messages are only
// concatenated, leaving duplicate detection to ValidateI18nCatalog so its precise
// errors are preserved. Conflicting non-empty descriptions for the same module
// are an error.
func MergeI18nCatalogs(catalogs ...I18nCatalog) (I18nCatalog, error) {
	var merged I18nCatalog
	for _, catalog := range catalogs {
		modules, err := mergeI18nModules(merged.Modules, catalog.Modules)
		if err != nil {
			return I18nCatalog{}, err
		}
		merged.Modules = modules
	}
	return merged, nil
}

func mergeI18nModules(dst, src []I18nModule) ([]I18nModule, error) {
	index := make(map[string]int, len(dst))
	for i, module := range dst {
		index[module.Name] = i
	}
	for _, module := range src {
		pos, ok := index[module.Name]
		if !ok {
			dst = append(dst, module)
			index[module.Name] = len(dst) - 1
			continue
		}
		combined, err := mergeI18nModule(dst[pos], module)
		if err != nil {
			return nil, err
		}
		dst[pos] = combined
	}
	return dst, nil
}

func mergeI18nModule(dst, src I18nModule) (I18nModule, error) {
	description, err := mergeI18nDescription(dst.Name, dst.Description, src.Description)
	if err != nil {
		return I18nModule{}, err
	}
	dst.Description = description
	dst.Messages = append(append([]I18nMessage{}, dst.Messages...), src.Messages...)
	children, err := mergeI18nModules(dst.Modules, src.Modules)
	if err != nil {
		return I18nModule{}, err
	}
	dst.Modules = children
	return dst, nil
}

func mergeI18nDescription(name, existing, incoming string) (string, error) {
	switch {
	case existing == "":
		return incoming, nil
	case incoming == "" || incoming == existing:
		return existing, nil
	default:
		return "", fmt.Errorf("conflicting descriptions for module %q", name)
	}
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
