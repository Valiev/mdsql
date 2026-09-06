package query

import "strings"

func matchesTag(tag, target string) bool {
	tag = strings.ToLower(tag)
	return tag == target || strings.HasPrefix(tag, target+"/")
}
