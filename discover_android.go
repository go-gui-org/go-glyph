//go:build android

package glyph

// systemFontDirs lists the Android system font directories. Family/style,
// generic aliases, and color/CJK/emoji fallbacks are read directly from each
// font's tables — no /system/etc/fonts.xml parse.
func systemFontDirs() []string {
	return []string{
		"/system/fonts",
		"/product/fonts", // vendor/product-partition fonts (Android 10+)
		"/system/font",   // some vendor images use the singular form
		"/data/fonts",    // runtime-installed downloadable fonts
	}
}

// platformAliases lists the Android families that may fill each generic
// alias, most preferred first. See aliasTable. /system/fonts holds a Noto
// Sans font per script (Adlam, Ahom, Arabic, …) that sorts before Roboto;
// whole-name matching keeps them out of sans-serif.
var platformAliases = aliasTable{
	"monospace": {
		"roboto mono", "droid sans mono", "noto sans mono",
		"cutive mono", "noto mono",
	},
	"serif":      {"noto serif", "droid serif"},
	"sans-serif": {"roboto", "noto sans", "droid sans"},
}

// platformGenerics are the families the generic names resolve to on
// Android. Concrete names match go-text's parsed family strings; if a
// family is absent, resolveFontPath degrades to the sans-serif alias.
var platformGenerics = genericNames{
	sans:  "Roboto",
	serif: "Noto Serif",
	mono:  "Roboto Mono",
}

// ensurePlatformDefaults makes sure a sans-serif face exists even if the
// directory walk found nothing (e.g. a restricted image). It fills only
// keys that discovery did not set, so real fonts take precedence. Missing
// serif/monospace resolve through resolveFontPath's sans-serif last resort.
func ensurePlatformDefaults(fontPaths map[string]string) {
	fillDefaultKeys(fontPaths, "/system/fonts/Roboto-Regular.ttf",
		"Roboto", "Roboto-Regular", "sans-serif")
}
