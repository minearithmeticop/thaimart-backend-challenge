package domain

import "testing"

func TestApplyUpdate(t *testing.T) {
	base := User{ID: "1", Name: "old", Email: "old@example.com"}

	name, email := "new", "new@example.com"
	cases := []struct {
		desc      string
		name      *string
		email     *string
		wantName  string
		wantEmail string
	}{
		{"name only", &name, nil, "new", "old@example.com"},
		{"email only", nil, &email, "old", "new@example.com"},
		{"both fields", &name, &email, "new", "new@example.com"},
		{"nothing", nil, nil, "old", "old@example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := base.ApplyUpdate(tc.name, tc.email)
			if got.Name != tc.wantName || got.Email != tc.wantEmail {
				t.Fatalf("ApplyUpdate() = %+v, want name=%q email=%q", got, tc.wantName, tc.wantEmail)
			}
		})
	}

	t.Run("does not mutate the receiver", func(t *testing.T) {
		base.ApplyUpdate(&name, &email)
		if base.Name != "old" || base.Email != "old@example.com" {
			t.Fatalf("receiver mutated: %+v", base)
		}
	})
}
