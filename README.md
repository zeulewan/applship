# applship

Apple app build, device, upload, and App Store Connect release CLI.

`applship` is intentionally smaller than Fastlane: it wraps Xcode tools, handles common simulator/device flows, and automates the App Store Connect release steps that usually require clicking through the web UI.

## Install

```bash
go install github.com/zeulewan/applship/cmd/applship@latest
```

From this checkout:

```bash
go install ./cmd/applship
```

## Quick Start

Create `.applship.json` in an app repo:

```json
{
  "project": "ios/App/App.xcodeproj",
  "scheme": "App",
  "bundleId": "com.example.app",
  "teamId": "TEAMID",
  "configuration": "Release"
}
```

Then:

```bash
applship doctor
applship sim list
applship build --simulator "iPhone 17"
applship install --device auto --app build/Build/Products/Debug-iphoneos/App.app
applship launch --simulator booted --bundle-id com.example.app
applship archive --version 1.0.0 --build 1
applship upload --archive build/applship/App-1.0.0.xcarchive
applship submit --version 1.0.0 --whats-new RELEASE_NOTES.md
applship status --version 1.0.0
```

## App Store Connect Auth

Set these env vars:

```bash
export APP_STORE_CONNECT_KEY_PATH=/path/to/AuthKey_KEYID.p8
export APP_STORE_CONNECT_KEY_ID=KEYID
export APP_STORE_CONNECT_ISSUER_ID=ISSUER_UUID
```

Secrets belong in your shell/keychain/CI secret store, not in `.applship.json`.

## Commands

- `doctor`: checks Xcode, `xcrun`, SDKs, project config, and App Store Connect auth.
- `sim list`: lists simulator devices.
- `sim boot NAME_OR_UDID`: boots a simulator.
- `build`: runs `xcodebuild build` for a simulator or device destination.
- `install`: installs an `.app` on a connected device or simulator.
- `launch`: launches an installed app on a connected device or simulator.
- `archive`: archives a generic iOS Release build and writes export options.
- `upload`: exports/uploads an archive to App Store Connect.
- `submit`: creates/updates an App Store version, attaches an eligible build, sets release notes, and submits for review.
- `status`: prints App Store versions and recent builds for the configured bundle id.
- `release`: archive, upload, wait for build processing, and submit.

## Design Notes

- Human progress goes to stderr; machine-readable output belongs on stdout for future `--json` use.
- CLI style follows the useful bits from `gogcli`: predictable commands, stderr for progress, stdout for results, and no secrets in project files.
- `manageAppVersionAndBuildNumber` is forced false on export so Apple/Xcode does not silently mutate build numbers.
- App Store submissions use the modern `reviewSubmissions` API flow: create submission, create item, then patch `submitted: true`.
