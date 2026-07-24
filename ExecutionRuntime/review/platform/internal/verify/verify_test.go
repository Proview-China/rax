package verify

import "testing"

func TestOfficialSlackAndJiraSignatureGoldensV1(t *testing.T) {
	slackSecret := []byte("8f742231b10e8888abcd99yyyzzz85a5")
	slackBody := []byte("token=xyzz0WbapA4vBCDEFasx0q6G&team_id=T1DC2JH3J&team_domain=testteamnow&channel_id=G8PSS9T3V&channel_name=foobar&user_id=U2CERLKJA&user_name=roadrunner&command=%2Fwebhook-collect&text=&response_url=https%3A%2F%2Fhooks.slack.com%2Fcommands%2FT1DC2JH3J%2F397700885554%2F96rGlfmibIGlgcZRskXaIFfN&trigger_id=398738663015.47445629121.803a0bc887a14d10d2c447fce8b6703c")
	base := append([]byte("v0:1531420618:"), slackBody...)
	if err := HMACSHA256(slackSecret, base, "v0=a2114d57b48eac39b9ad189dd8316235a7b4a8d21a10bd27519666489c69b503", "v0="); err != nil {
		t.Fatalf("Slack official golden failed: %v", err)
	}
	if err := HMACSHA256([]byte("It's a Secret to Everybody"), []byte("Hello World!"), "sha256=a4771c39fbe90f317c7824e83ddef3caae9cb3d976c214ace1f2937e133263c9", "sha256="); err != nil {
		t.Fatalf("Jira official golden failed: %v", err)
	}
}
