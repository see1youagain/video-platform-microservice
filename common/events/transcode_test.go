package events

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidateTranscodeRequest(t *testing.T) {
	for _, hash := range []string{strings.Repeat("a", 32), strings.Repeat("A", 64)} {
		input := []string{"720p", "360p", "720p", "1080p", "480p"}
		got, err := ValidateTranscodeRequest(hash, input)
		if err != nil || !reflect.DeepEqual(got, []string{"720p", "360p", "1080p", "480p"}) {
			t.Fatalf("got=%v err=%v", got, err)
		}
		if len(input) != 5 || input[2] != "720p" {
			t.Fatal("request was mutated")
		}
	}
	cases := []struct {
		hash     string
		profiles []string
	}{
		{"", []string{"360p"}}, {"../video", []string{"360p"}}, {strings.Repeat("a", 31), []string{"360p"}},
		{strings.Repeat("z", 32), []string{"360p"}}, {strings.Repeat("a", 32), nil},
		{strings.Repeat("a", 32), []string{"4k"}}, {strings.Repeat("a", 32), []string{"360P"}},
		{strings.Repeat("a", 32), []string{"../360p"}}, {strings.Repeat("a", 32), make([]string, 17)},
	}
	for _, tc := range cases {
		if _, err := ValidateTranscodeRequest(tc.hash, tc.profiles); err == nil {
			t.Fatalf("accepted invalid input: %+v", tc)
		}
	}
}

func TestValidTaskID(t *testing.T) {
	for _, id := range []string{"tc_123-ABC", strings.Repeat("x", 64)} {
		if !ValidTaskID(id) {
			t.Fatalf("rejected %q", id)
		}
	}
	for _, id := range []string{"", strings.Repeat("x", 65), "../task", "a/b", "a\\b", "a b", "任务", "task\n"} {
		if ValidTaskID(id) {
			t.Fatalf("accepted %q", id)
		}
	}
}
