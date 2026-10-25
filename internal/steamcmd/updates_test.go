package steamcmd

import (
	"strings"
	"testing"
)

func TestUpdateResponseRequiresPositiveEvidence(t *testing.T) {
	for _, tc := range []struct {
		body         string
		update, fail bool
	}{
		{`{"response":{"success":true,"up_to_date":true}}`, false, false},
		{`{"response":{"success":true,"up_to_date":false,"required_version":756039}}`, true, false},
		{`{"response":{"success":false}}`, false, true},
		{`{"response":{"success":true}}`, false, true},
		{`{"response":{"success":true,"up_to_date":false}}`, false, true},
		{`not json`, false, true},
	} {
		got, err := parseUpdate(strings.NewReader(tc.body))
		if got != tc.update || (err != nil) != tc.fail {
			t.Fatalf("%s: %t %v", tc.body, got, err)
		}
	}
}
