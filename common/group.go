package common

import "strings"

const (
	// MaxGroupNameLength is the maximum length of a single group name in a
	// comma-separated group list.
	MaxGroupNameLength = 64
	// MaxUserGroupColumnLength matches the users.group column width
	// (varchar(1024)) that stores the comma-separated multi-group list.
	MaxUserGroupColumnLength = 1024
)

// SplitGroupList splits a comma-separated group list (e.g. a multi-group user
// or channel) into trimmed, non-empty, de-duplicated group names, preserving
// the original order.
func SplitGroupList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	groups := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups
}

// PrimaryGroup returns the first group of a comma-separated list, falling back
// to "default" when the list contains no valid group.
func PrimaryGroup(s string) string {
	if groups := SplitGroupList(s); len(groups) > 0 {
		return groups[0]
	}
	return "default"
}

// NormalizeGroupList canonicalizes a comma-separated group list by trimming
// whitespace and removing empty duplicates. It returns "default" when nothing
// valid remains so a user always keeps at least one group.
func NormalizeGroupList(s string) string {
	groups := SplitGroupList(s)
	if len(groups) == 0 {
		return "default"
	}
	return strings.Join(groups, ",")
}
