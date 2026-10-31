// Package useragent builds the identity every outbound request must carry.
package useragent

import "fmt"

const (
	// Contact is public. It must never be a personal address.
	Contact = "bot@dhruvgoyal.tech"
	// AboutURL explains the bot to anyone who sees it in their logs.
	AboutURL = "https://rolerank.dhruvgoyal.tech/about/bot"
)

// String returns the User-Agent for a build version.
func String(version string) string {
	return fmt.Sprintf("RoleRankBot/%s (+%s; %s)", version, AboutURL, Contact)
}
