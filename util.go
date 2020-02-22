package blog

import "strings"

func Slugify(title string) string {

	parts := strings.FieldsFunc(strings.ToLower(title), func(c rune) bool {
		return c == '/' ||
			c == '\\' || c == '.' || c == ',' ||
			c == '_' || c == '!' || c == '\'' ||
			c == '"' || c == ':' || c == ';' ||
			c == '&' || c == '`' || c == '$' ||
			c == '#' || c == '@' || c == '(' ||
			c == ')' || c == '=' || c == '~' ||
			c == '。' || c == '，' || c == '！' ||
			c == '【' || c == '】' || c == '、' ||
			c == '·' || c == '「' || c == '」' ||
			c == '｜' || c == '|' || c == '%' ||
			c == '：' || c == '；' ||
			c == '?' || c == ' ' || c == '-'
	})

	return strings.Join(parts, "-")
}

func SplitTags(tags string) []string {

	parts := strings.FieldsFunc(tags, func(c rune) bool {
		return c == '/' ||
			c == '\\' || c == '.' || c == ',' ||
			c == '_' || c == '!' || c == '\'' ||
			c == '"' || c == ':' || c == ';' ||
			c == '&' || c == '`' || c == '$' ||
			c == '#' || c == '@' || c == '(' ||
			c == ')' || c == '=' || c == '~' ||
			c == '。' || c == '，' || c == '！' ||
			c == '【' || c == '】' || c == '、' ||
			c == '·' || c == '「' || c == '」' ||
			c == '｜' || c == '|' || c == '%' ||
			c == '：' || c == '；' ||
			c == '?' || c == ' ' || c == '-'
	})

	return parts
}
