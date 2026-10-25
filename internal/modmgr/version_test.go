package modmgr

import "testing"

func TestClientOnlyFlagUsesBooleanTopLevelDeclaration(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   bool
	}{
		{`name="client" client_only_mod=true api_version=10`, true},
		{`local strings={name="种植计算器"} name=strings.name all_clients_require_mod=false client_only_mod=true local keys={"K"} for i=1,12 do keys[#keys+1]="F"..i end configuration_options={}`, true},
		{`client_only_mod=true if locale == "zh" then name="种植计算器" end`, true},
		{`client_only_mod=true and false local keys={}`, false},
		{"client_only_mod = true -- 客户端\n", true},
		{`client_only_mod=false all_clients_require_mod=true`, false},
		{`server_only_mod=true`, false},
		{`all_clients_require_mod=true`, false},
		{`client_only_mod="true"`, false},
		{`description="client_only_mod=true"`, false},
		{`configuration_options={{client_only_mod=true}}`, false},
		{"--[[\nclient_only_mod=true\n]]", false},
		{`client_only_mod=true client_only_mod=false`, false},
		{`client_only_mod=GetClientOnly()`, false},
	} {
		if got := extractModInfoLiteral(tc.source, "client_only_mod") == "true"; got != tc.want {
			t.Errorf("%q: got %t, want %t", tc.source, got, tc.want)
		}
	}
}

func TestModVersionUsesDeclaredLiteralWithoutGuessing(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"\ufeffversion = '1.2.3-beta' -- release\r\n", "1.2.3-beta"},
		{`version = "v2026.10";`, "v2026.10"},
		{`version = 2.1`, "2.1"},
		{"-- version = 'old'\n--[[\nversion = 'also old'\n]]\nversion = '3'", "3"},
		{"configuration_options = {{\nversion = 'wrong'\n}}\nversion = '4'", "4"},
		{`api_version = 10`, ""},
		{`version = GetVersion()`, ""},
		{`version = "1." .. build`, ""},
		{`version = true`, ""},
		{`name = "No version"`, ""},
		{`version="2.16" local options={}`, "2.16"},
		{`name = " Simple Health Bar DST" version = "2.16" description = "Version:DST"..version.."\nHealth bar" author = "DYC" api_version = 0x6`, "2.16"},
		{`description = "version = 'fake'" version = '2.16'`, "2.16"},
		{"description = [[\nversion = 'fake'\n]]\nversion = '2.16'", "2.16"},
		{"--[=[\nversion = 'fake'\n]=]\nversion = '2.16'", "2.16"},
		{`configuration_options = {{version = 'fake'}} version = '2.16'`, "2.16"},
		{`function example() version = 'fake' end version = '2.16'`, "2.16"},
		{`local version = 'fake'`, ""},
		{`info.version = 'fake'`, ""},
	} {
		if got := extractModInfoVersion(tc.source); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.source, got, tc.want)
		}
	}
}
