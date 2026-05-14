package applship

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeExportOptions(path string, cfg Config, destination string) error {
	if destination == "" {
		destination = "upload"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	team := ""
	if cfg.TeamID != "" {
		team = fmt.Sprintf("    <key>teamID</key>\n    <string>%s</string>\n", cfg.TeamID)
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>method</key>
    <string>app-store-connect</string>
    <key>manageAppVersionAndBuildNumber</key>
    <false/>
%s    <key>signingStyle</key>
    <string>automatic</string>
    <key>stripSwiftSymbols</key>
    <true/>
    <key>destination</key>
    <string>%s</string>
</dict>
</plist>
`, team, destination)
	return os.WriteFile(path, []byte(content), 0o644)
}
