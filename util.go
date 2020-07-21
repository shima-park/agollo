package agollo

import "strings"

func normalizeURL(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

	return strings.TrimSuffix(url, "/")
}

func splitCommaSeparatedURL(s string) []string {
	var urls []string
	for _, url := range strings.Split(s, ",") {
		urls = append(urls, normalizeURL(strings.TrimSpace(url)))
	}

	return urls
}

func stringInSlice(t string, ss []string) bool {
	for _, s := range ss {
		if s == t {
			return true
		}
	}
	return false
}

func nonEmptyString(def string, ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return def
}
