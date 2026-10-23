package buildinfo

import (
	"encoding/json"
	"os"
	"testing"
)

func TestReleaseVersionMatchesWails(t *testing.T) {
	raw, err := os.ReadFile("../../wails.json")
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if info.Info.ProductVersion != Version {
		t.Fatal("diagnostics version differs from Wails release version")
	}
}
