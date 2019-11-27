package agollo

import "strings"

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

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
