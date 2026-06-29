package auth

import "testing"

func TestNormalizeUsers(t *testing.T) {
	users := NormalizeUsers([]string{" Dibou ", "dibou", "", "Other"})
	if len(users) != 2 {
		t.Fatalf("unexpected user count: %d", len(users))
	}
	if users[0] != "dibou" || users[1] != "other" {
		t.Fatalf("unexpected normalized users: %#v", users)
	}
}
