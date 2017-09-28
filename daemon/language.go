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

	lang = strings.ToLower(lang)
	lang = strings.Replace(lang, " ", "-", -1)
	lang = strings.Replace(lang, "+", "p", -1)
	lang = strings.Replace(lang, "#", "sharp", -1)
	return lang
}
