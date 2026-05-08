package build

import "testing"

func TestKeyFromSourceURI(t *testing.T) {
	cases := []struct {
		name    string
		uri     string
		want    string
		wantErr bool
	}{
		{"happy", "s3://submissions/team1/abc/source.tar.gz", "team1/abc/source.tar.gz", false},
		{"deep", "s3://b/x/y/z.tgz", "x/y/z.tgz", false},
		{"non-s3", "https://example.com/file", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := keyFromSourceURI(tc.uri)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short: got %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcde" {
		t.Errorf("long: got %q", got)
	}
}
