package main

import (
	"flag"
	"io"
	"maps"
	"sort"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

type (
	i18nCatalog = manifest.I18nCatalog
	i18nModule  = manifest.I18nModule
)

func runI18n(args []string) error {
	fs := flag.NewFlagSet("i18n", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "catalog", "manifest input path (directory of fragments or single file)")
	output := fs.String("output", "generated.go", "generated Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return generateI18n(*input, *output)
}

func generateI18n(inputPath, outputPath string) error {
	mf, err := manifest.LoadI18nCatalog(inputPath)
	if err != nil {
		return err
	}
	if err := manifest.ValidateI18nCatalog(mf); err != nil {
		return err
	}

	src, err := renderI18nManifest(mf)
	if err != nil {
		return err
	}
	return writeGeneratedFile(outputPath, src)
}

func renderI18nManifest(mf i18nCatalog) ([]byte, error) {
	messages := flattenI18nMessages(mf.Modules, nil)
	sort.Slice(messages, func(i, j int) bool { return messages[i].Code < messages[j].Code })

	view := i18nFileView{}
	for _, message := range messages {
		view.Messages = append(view.Messages, i18nMessageView{
			ConstantName: message.ConstantName,
			Code:         message.Code,
			English:      message.Translations["en"],
		})
	}
	return renderTemplate("i18n.go.tmpl", view)
}

type i18nFileView struct {
	Messages []i18nMessageView
}

type i18nMessageView struct {
	ConstantName string
	Code         string
	English      string
}

type flattenedI18nMessage struct {
	Code         string
	ConstantName string
	Translations map[string]string
}

func flattenI18nMessages(modules []i18nModule, modulePath []string) []flattenedI18nMessage {
	out := make([]flattenedI18nMessage, 0)
	for _, module := range modules {
		currentPath := append(append([]string{}, modulePath...), module.Name)
		for _, message := range module.Messages {
			out = append(out, flattenedI18nMessage{
				Code:         manifest.I18nCodeFor(currentPath, message.Name),
				ConstantName: manifest.I18nConstantName(currentPath, message.Name),
				Translations: maps.Clone(message.Translations),
			})
		}
		if len(module.Modules) > 0 {
			out = append(out, flattenI18nMessages(module.Modules, currentPath)...)
		}
	}
	return out
}
