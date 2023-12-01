package main

import "testing"

func TestValidateWebhook(t *testing.T) {
	ok, err := validateWebhook(
		"sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17",
		"It's a Secret to Everybody",
		[]byte("Hello, World!"),
	)
	if err != nil || !ok {
		t.Fail()
	}
}
