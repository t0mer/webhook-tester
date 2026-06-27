package notify

import "testing"

func TestSendEmptyURLIsNoOp(t *testing.T) {
	if err := Send("", "title", "msg"); err != nil {
		t.Errorf("empty URL should be a no-op, got %v", err)
	}
}

func TestValid(t *testing.T) {
	if !Valid("") {
		t.Error("empty URL should be considered valid (optional)")
	}
	if Valid("not-a-shoutrrr-url") {
		t.Error("garbage URL should be invalid")
	}
	if !Valid("logger://") {
		t.Error("logger URL should be valid")
	}
}
