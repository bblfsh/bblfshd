package daemon

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/src-d/enry.v1"
)

// GetLanguage detects the language of a file and returns it in a normalized
// form.
func GetLanguage(filename string, content []byte) string {
	totalEnryCalls.Add(1)
	defer prometheus.NewTimer(enryDetectLatency).ObserveDuration()

	lang := enry.GetLanguage(filename, content)
	if lang == enry.OtherLanguage {
		enryOtherResults.Add(1)
		return lang
	}
	enryLangResults.WithLabelValues(lang).Add(1)
	lang = normalize(lang)
	return lang
}

// normalize maps enry language names to the bblfsh ones.
// TODO(bzz): remove this as soon as language aliases are supported in bblfsh
// driver manifest.
func normalize(languageName string) string {
	lang := strings.ToLower(languageName)
	lang = strings.Replace(lang, " ", "-", -1)
	lang = strings.Replace(lang, "+", "p", -1)
	lang = strings.Replace(lang, "#", "sharp", -1)
	return lang
}
