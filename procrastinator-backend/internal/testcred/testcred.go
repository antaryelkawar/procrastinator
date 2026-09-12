// Package testcred holds the shared, test-only Basic Auth credential set used
// by the API integration suites (api/ and e2e/) so their previously
// unauthenticated call sites keep passing behind the auth middleware
// (add-basic-auth task 5.1).
//
// It is imported only from *_test.go files, so nothing here is linked into the
// production binary; the internal/ path additionally makes it unimportable
// from outside this module.
package testcred

import (
	"encoding/base64"

	"procrastinator-backend/config"
)

// Creds is the fixture credential set every API integration test uses. It
// satisfies the PROCRASTINATOR_BASIC_AUTH_USERS grammar bounds (user 1-64
// UTF-8 runes, pass 8-128 UTF-8 runes) so it could be fed through config.Load
// unchanged.
var Creds = []config.Credential{
	{User: "fixture-user", Pass: "fixture-password-1"},
}

// Header returns the Authorization header value for Creds[0] — the pair the
// test Servers are constructed with.
func Header() string { return HeaderFor(Creds[0]) }

// HeaderFor builds the Authorization header value ("Basic <base64(user:pass)>")
// for one credential pair, for tests that need a deliberately wrong pair.
func HeaderFor(c config.Credential) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.User+":"+c.Pass))
}
