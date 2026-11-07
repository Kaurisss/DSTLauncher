package diagnostics

import "testing"

func TestEvidenceOnlyAttributesFailingModStack(t *testing.T) {
	lines := []string{
		"Loading mod workshop-999999", "Server started",
		`LUA ERROR stack traceback:`,
		`mods/workshop-123456/scripts/foo.lua:10: attempt to index nil`,
		"Loading mod workshop-888888",
	}
	got := Analyze(lines)
	if len(got) != 1 || got[0].Code != "lua" || len(got[0].Mods) != 1 || got[0].Mods[0].ID != "workshop-123456" {
		t.Fatalf("错误归因: %+v", got)
	}
	if len(Analyze([]string{"Loading mod workshop-999999", "FinishDownloadingServerMods Complete"})) != 0 {
		t.Fatal("正常日志被识别为错误")
	}
}

func TestDiagnosticCategoriesAndSecretRedaction(t *testing.T) {
	for _, tc := range []struct{ line, code string }{
		{"Invalid server token: private-token", "token"},
		{"Socket bind failed: Address already in use", "port"},
		{"DownloadPublishedFile failed for item 123456789", "download"},
		{"out of memory", "memory"},
		{"Failed to connect to master", "master"},
		{"server is out of date", "version"},
	} {
		got := Analyze(Redact([]string{tc.line}, "private-token"))
		if len(got) != 1 || got[0].Code != tc.code {
			t.Fatal(tc, got)
		}
		if tc.code == "token" && got[0].Evidence[0] != "Invalid server token: [已隐藏]" {
			t.Fatal(got)
		}
		if tc.code == "download" && (len(got[0].Mods) != 1 || got[0].Mods[0].ID != "workshop-123456789") {
			t.Fatal(got)
		}
	}
}
