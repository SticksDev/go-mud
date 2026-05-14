package shell

import "regexp"

var colorTagRe = regexp.MustCompile(`</?color[^>]*>`)

// StripColors removes hackmud color tags from text.
// Example: <color=#FFFFFFFF>>><color=#9B9B9BFF>flush</color></color> becomes >>flush
func StripColors(raw string) string {
	return colorTagRe.ReplaceAllString(raw, "")
}
