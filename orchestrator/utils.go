package main

import "strings"

// sanitizeKey prepares a string to be used as a path component.
func sanitizeKey(key string) string {
	lower := strings.ToLower(key)
	return strings.ReplaceAll(lower, " ", "_")
}

// isAssetPath checks if a filename is a relative asset path and not a full URL.
func isAssetPath(filename string) bool {
	return !strings.HasPrefix(filename, "http://") && !strings.HasPrefix(filename, "https://")
}
