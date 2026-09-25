package internal

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestJSONShapes(t *testing.T) {
	env := newTestEnv(t, twoProfiles)
	mustMkdir(t, filepath.Join(env.profilesBase(), "work"))
	stubKeychain(t, true)
	t.Setenv("CLAUDE_PROFILE", "work")
	buf := captureOutput(t)

	if err := PrintJSON(ListProfiles(env.load(t), env.profilesBase())); err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &list); err != nil {
		t.Fatalf("list JSON invalid: %v\n%s", err, buf.String())
	}
	if len(list) != 2 || list[1]["name"] != "work" || list[1]["authenticated"] != true || list[1]["credential_source"] != "keychain" || list[1]["current"] != true {
		t.Errorf("unexpected list JSON: %v", list)
	}
	if _, ok := list[0]["account"]; ok {
		t.Error("empty account must be omitted")
	}

	buf.Reset()
	if err := PrintJSON(Which(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	var which map[string]any
	json.Unmarshal(buf.Bytes(), &which)
	if which["active"] != true || which["profile"] != "work" || which["source"] != "env" {
		t.Errorf("unexpected which JSON: %v", which)
	}

	buf.Reset()
	if err := PrintJSON(CredentialReport(env.load(t), env.profilesBase())); err != nil {
		t.Fatal(err)
	}
	var creds []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &creds); err != nil {
		t.Fatal(err)
	}
	if creds[1]["authenticated"] != true || creds[1]["source"] != "keychain" {
		t.Errorf("unexpected credentials JSON: %v", creds)
	}
	if _, ok := creds[0]["expires_at"]; ok {
		t.Error("zero expiry must be omitted")
	}
	if creds[0]["authenticated"] != true {
		t.Errorf("stubbed keychain should authenticate every profile: %v", creds[0])
	}

	buf.Reset()
	if err := PrintJSON(DoctorReportFor(env.load(t), env.profilesBase(), DoctorOptions{})); err != nil {
		t.Fatal(err)
	}
	var doctor map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doctor); err != nil {
		t.Fatal(err)
	}
	checks, _ := doctor["checks"].([]any)
	if len(checks) == 0 || checks[0].(map[string]any)["status"] == nil || doctor["ok"] == nil || doctor["errors"] == nil {
		t.Errorf("unexpected doctor JSON: %v", doctor)
	}
}
