package daemon

import (
	"strings"

	"gopkg.in/src-d/enry.v1"
)

// GetLanguage detects the language of a file and returns it in a normalized
// form.
func GetLanguage(filename string, content []byte) string {
	lang := enry.GetLanguage(filename, content)
	if lang == enry.OtherLanguage {
		return lang
	}

	return normalize(lang)
}

// normalize maps enry language names to the bblfsh ones.
// TODO(bzz): remove this as soon as languafe_aliaces are supported in bblfsh
// driver manifest.
func normalize(languageName string) string {
	lang := strings.ToLower(languageName)
	lang = strings.Replace(lang, " ", "-", -1)
	lang = strings.Replace(lang, "+", "p", -1)
	lang = strings.Replace(lang, "#", "sharp", -1)
	return lang
}
