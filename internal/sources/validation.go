package sources

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	youtubeChannelIDRE = regexp.MustCompile(`^UC[0-9A-Za-z_-]{22}$`)
	xHandleRE          = regexp.MustCompile(`^@?[A-Za-z0-9_]{1,15}$`)
)

// ValidateIdentifier checks an identifier against the rules for its type and
// returns a normalised form (e.g. X handles lose any leading @).
func ValidateIdentifier(t Type, raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", errors.New("identifier is required")
	}
	switch t {
	case TypeRSS, TypeWeb, TypeSubstack:
		u, err := url.Parse(id)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", errors.New("identifier must be an absolute http(s) URL")
		}
		return id, nil
	case TypeYouTubeChannel:
		if !youtubeChannelIDRE.MatchString(id) {
			return "", errors.New("identifier must be a YouTube channel ID (e.g. UCxxxxxxxxxxxxxxxxxxxxxx)")
		}
		return id, nil
	case TypeXHandle:
		if !xHandleRE.MatchString(id) {
			return "", errors.New("identifier must be an X/Twitter handle (1–15 chars, letters/digits/underscore)")
		}
		return strings.TrimPrefix(id, "@"), nil
	}
	return "", errors.New("unknown source type")
}
