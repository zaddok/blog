package blog

import "strings"

func Slugify(title string) string {

	parts := strings.FieldsFunc(title, func(c rune) bool {
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
