package utils

import (
	"fmt"
	"regexp"
)

const (
	VersionPattern   = `^[0-9]+\.[0-9]+\.[0-9]+(-.+)?$`
	VersionTagLatest = "latest"
)

func IsValidVersion(version string) bool {
	return regexp.MustCompile(VersionPattern).Match([]byte(version))
}

func ToVersionTag(version string) string {
	if IsValidVersion(version) {
		return fmt.Sprintf("v%s", version)
	}

	return VersionTagLatest
}
