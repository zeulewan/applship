package applship

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const usage = `applship - Apple app build, device, upload, and App Store Connect release CLI

Usage:
  applship init --scheme SCHEME --bundle-id BUNDLE_ID [--team-id TEAMID]
  applship doctor
  applship sim list
  applship sim boot NAME_OR_UDID
  applship build [--simulator NAME_OR_UDID | --device auto]
  applship install --app PATH [--simulator NAME_OR_UDID | --device auto]
  applship launch --bundle-id BUNDLE_ID [--simulator booted | --device UDID]
  applship archive --version X.Y.Z [--build N]
  applship upload --archive PATH
  applship submit --version X.Y.Z [--build-number N] [--whats-new FILE] [--no-submit]
  applship price free [--bundle-id BUNDLE_ID] [--territory USA]
  applship status
  applship app create --name NAME --bundle-id BUNDLE_ID [--sku SKU]
  applship release --version X.Y.Z [--build N] [--whats-new FILE] [--submit]
`

func Main(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usage)
		return 0
	}
	cfg, err := LoadConfig()
	if err != nil {
		return fail(err)
	}
	var cmdErr error
	switch args[0] {
	case "init":
		cmdErr = initConfig(cfg, args[1:])
	case "doctor":
		cmdErr = doctor(cfg)
	case "sim":
		cmdErr = sim(args[1:])
	case "build":
		cmdErr = build(cfg, args[1:])
	case "install":
		cmdErr = install(args[1:])
	case "launch":
		cmdErr = launch(cfg, args[1:])
	case "archive":
		cmdErr = archive(cfg, args[1:])
	case "upload":
		cmdErr = upload(cfg, args[1:])
	case "submit":
		cmdErr = submitCmd(cfg, args[1:])
	case "price":
		cmdErr = priceCmd(cfg, args[1:])
	case "status":
		cmdErr = status(cfg, args[1:])
	case "app":
		cmdErr = appCmd(cfg, args[1:])
	case "release":
		cmdErr = release(cfg, args[1:])
	default:
		cmdErr = fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
	if cmdErr != nil {
		return fail(cmdErr)
	}
	return 0
}

func priceCmd(cfg Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: applship price free")
	}
	switch args[0] {
	case "free":
		return priceFree(cfg, args[1:])
	default:
		return fmt.Errorf("unknown price command %q", args[0])
	}
}

func priceFree(cfg Config, args []string) error {
	fs := flag.NewFlagSet("price free", flag.ContinueOnError)
	bundleID := fs.String("bundle-id", cfg.BundleID, "Bundle ID")
	territory := fs.String("territory", "USA", "Base App Store territory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundleID == "" {
		return fmt.Errorf("missing bundle id; set bundleId in .applship.json or pass --bundle-id")
	}
	client, err := NewASCClientFromEnv()
	if err != nil {
		return err
	}
	appID, err := client.FindAppID(*bundleID)
	if err != nil {
		return err
	}
	scheduleID, err := client.SetFreePrice(appID, *territory)
	if err != nil {
		return err
	}
	fmt.Printf("price free app=%s territory=%s schedule=%s\n", appID, *territory, scheduleID)
	return nil
}

func appCmd(cfg Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: applship app create")
	}
	switch args[0] {
	case "create":
		return appCreate(cfg, args[1:])
	default:
		return fmt.Errorf("unknown app command %q", args[0])
	}
}

func appCreate(cfg Config, args []string) error {
	fs := flag.NewFlagSet("app create", flag.ContinueOnError)
	name := fs.String("name", "", "App Store app name")
	bundleID := fs.String("bundle-id", cfg.BundleID, "Bundle ID")
	sku := fs.String("sku", "", "SKU")
	locale := fs.String("locale", "en-US", "Primary locale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("missing --name")
	}
	if *bundleID == "" {
		return fmt.Errorf("missing --bundle-id")
	}
	client, err := NewASCClientFromEnv()
	if err != nil {
		return err
	}
	if appID, err := client.LookupAppID(*bundleID); err != nil {
		return err
	} else if appID != "" {
		fmt.Printf("app exists app=%s bundleId=%s\n", appID, *bundleID)
		return nil
	}
	bundleResourceID, err := client.EnsureBundleID(*name, *bundleID)
	if err != nil {
		return err
	}
	appID, err := client.CreateApp(*name, bundleResourceID, *sku, *locale)
	if err != nil {
		return err
	}
	fmt.Printf("created app=%s bundleResource=%s bundleId=%s\n", appID, bundleResourceID, *bundleID)
	return nil
}

func initConfig(cfg Config, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	project := fs.String("project", cfg.Project, "Xcode project")
	workspace := fs.String("workspace", cfg.Workspace, "Xcode workspace")
	scheme := fs.String("scheme", cfg.Scheme, "Xcode scheme")
	bundleID := fs.String("bundle-id", cfg.BundleID, "Bundle ID")
	teamID := fs.String("team-id", cfg.TeamID, "Apple Developer team ID")
	configuration := fs.String("configuration", cfg.Configuration, "Release configuration")
	outputDir := fs.String("out", cfg.OutputDir, "Output directory")
	force := fs.Bool("force", false, "Overwrite .applship.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(".applship.json"); err == nil && !*force {
		return fmt.Errorf(".applship.json already exists; use --force to overwrite")
	}
	if *scheme == "" {
		return fmt.Errorf("missing --scheme")
	}
	next := Config{
		Project:       *project,
		Workspace:     *workspace,
		Scheme:        *scheme,
		BundleID:      *bundleID,
		TeamID:        *teamID,
		Configuration: *configuration,
		OutputDir:     *outputDir,
	}
	b, err := marshalConfig(next)
	if err != nil {
		return err
	}
	if err := os.WriteFile(".applship.json", b, 0o644); err != nil {
		return err
	}
	fmt.Println(".applship.json")
	return nil
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return 1
}

func doctor(cfg Config) error {
	check := func(label, tool string) {
		if exists(tool) {
			fmt.Printf("ok   %s\n", label)
		} else {
			fmt.Printf("miss %s\n", label)
		}
	}
	check("xcodebuild", "xcodebuild")
	check("xcrun", "xcrun")
	if out, err := output("xcodebuild", "-version"); err == nil {
		fmt.Print(out)
	} else {
		fmt.Println(err)
	}
	if out, err := output("xcodebuild", "-showsdks"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "iphoneos") || strings.Contains(line, "iphonesimulator") {
				fmt.Println(line)
			}
		}
	}
	fmt.Printf("project=%s workspace=%s scheme=%s bundleId=%s teamId=%s\n", cfg.Project, cfg.Workspace, cfg.Scheme, cfg.BundleID, redacted(cfg.TeamID))
	if _, err := NewASCClientFromEnv(); err != nil {
		fmt.Println("asc auth=missing")
	} else {
		fmt.Println("asc auth=ok")
	}
	return nil
}

func redacted(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "..." + value[len(value)-2:]
}

func sim(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: applship sim list|boot")
	}
	switch args[0] {
	case "list":
		return run("xcrun", "simctl", "list", "devices", "available")
	case "boot":
		if len(args) < 2 {
			return fmt.Errorf("usage: applship sim boot NAME_OR_UDID")
		}
		return run("xcrun", "simctl", "boot", strings.Join(args[1:], " "))
	default:
		return fmt.Errorf("unknown sim command %q", args[0])
	}
}

func build(cfg Config, args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	scheme := fs.String("scheme", cfg.Scheme, "Xcode scheme")
	configuration := fs.String("configuration", "Debug", "Xcode configuration")
	simulator := fs.String("simulator", "", "Simulator name or UDID")
	device := fs.String("device", "", "Device destination; use auto for generic iOS")
	derivedData := fs.String("derived-data", "", "DerivedData path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg.Scheme = *scheme
	if err := cfg.ValidateBuild(); err != nil {
		return err
	}
	dest := destination(*simulator, *device)
	if dest == "" {
		dest = "generic/platform=iOS Simulator"
	}
	xb := append([]string{}, cfg.XcodeContainerArgs()...)
	xb = append(xb, "-scheme", cfg.Scheme, "-configuration", *configuration, "-destination", dest)
	if *derivedData != "" {
		xb = append(xb, "-derivedDataPath", *derivedData)
	}
	xb = append(xb, "build")
	return run("xcodebuild", xb...)
}

func destination(simulator, device string) string {
	if simulator != "" {
		if strings.Contains(simulator, "-") && len(simulator) > 20 {
			return "id=" + simulator
		}
		return "platform=iOS Simulator,name=" + simulator
	}
	if device != "" {
		if device == "auto" {
			return "generic/platform=iOS"
		}
		return "id=" + device
	}
	return ""
}

func install(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	app := fs.String("app", "", "Path to .app")
	simulator := fs.String("simulator", "", "Simulator name, UDID, or booted")
	device := fs.String("device", "", "Device UDID or auto")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *app == "" {
		return fmt.Errorf("missing --app")
	}
	if *device != "" {
		dev := *device
		if dev == "auto" {
			dev = ""
		}
		cmd := []string{"devicectl", "device", "install", "app"}
		if dev != "" {
			cmd = append(cmd, "--device", dev)
		}
		cmd = append(cmd, *app)
		return run("xcrun", cmd...)
	}
	sim := *simulator
	if sim == "" || sim == "booted" {
		sim = "booted"
	}
	return run("xcrun", "simctl", "install", sim, *app)
}

func launch(cfg Config, args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	bundleID := fs.String("bundle-id", cfg.BundleID, "Bundle ID")
	simulator := fs.String("simulator", "", "Simulator name, UDID, or booted")
	device := fs.String("device", "", "Device UDID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundleID == "" {
		return fmt.Errorf("missing --bundle-id")
	}
	if *device != "" {
		return run("xcrun", "devicectl", "device", "process", "launch", "--device", *device, *bundleID)
	}
	sim := *simulator
	if sim == "" {
		sim = "booted"
	}
	return run("xcrun", "simctl", "launch", sim, *bundleID)
}

func archive(cfg Config, args []string) error {
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	version := fs.String("version", "", "Marketing version")
	buildNumber := fs.String("build", "", "Build number")
	scheme := fs.String("scheme", cfg.Scheme, "Xcode scheme")
	configuration := fs.String("configuration", cfg.Configuration, "Xcode configuration")
	outDir := fs.String("out", cfg.OutputDir, "Output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg.Scheme = *scheme
	cfg.Configuration = *configuration
	if err := cfg.ValidateBuild(); err != nil {
		return err
	}
	if *version == "" {
		return fmt.Errorf("missing --version")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	archivePath := filepath.Join(*outDir, cfg.Scheme+"-"+*version+".xcarchive")
	xb := append([]string{}, cfg.XcodeContainerArgs()...)
	xb = append(xb, "-scheme", cfg.Scheme, "-configuration", cfg.Configuration, "-archivePath", archivePath, "-destination", "generic/platform=iOS", "-allowProvisioningUpdates")
	if *buildNumber != "" {
		xb = append(xb, "CURRENT_PROJECT_VERSION="+*buildNumber)
	}
	xb = append(xb, "archive")
	if err := run("xcodebuild", xb...); err != nil {
		return err
	}
	fmt.Println(archivePath)
	return nil
}

func upload(cfg Config, args []string) error {
	fs := flag.NewFlagSet("upload", flag.ContinueOnError)
	archivePath := fs.String("archive", "", "Archive path")
	exportPath := fs.String("export-path", "", "Export path")
	exportOptions := fs.String("export-options", "", "ExportOptions.plist path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *archivePath == "" {
		return fmt.Errorf("missing --archive")
	}
	outDir := cfg.OutputDir
	if outDir == "" {
		outDir = "build/applship"
	}
	if *exportPath == "" {
		*exportPath = filepath.Join(outDir, "export-"+strings.TrimSuffix(filepath.Base(*archivePath), ".xcarchive"))
	}
	if *exportOptions == "" {
		*exportOptions = filepath.Join(outDir, "ExportOptions.plist")
	}
	if err := writeExportOptions(*exportOptions, cfg, "upload"); err != nil {
		return err
	}
	xb := []string{"-exportArchive", "-archivePath", *archivePath, "-exportOptionsPlist", *exportOptions, "-exportPath", *exportPath, "-allowProvisioningUpdates"}
	if keyArgs := ascXcodeAuthArgs(); len(keyArgs) > 0 {
		xb = append(xb, keyArgs...)
	}
	return run("xcodebuild", xb...)
}

func ascXcodeAuthArgs() []string {
	keyPath := os.Getenv("APP_STORE_CONNECT_KEY_PATH")
	keyID := os.Getenv("APP_STORE_CONNECT_KEY_ID")
	issuerID := os.Getenv("APP_STORE_CONNECT_ISSUER_ID")
	if keyPath == "" && keyID == "" && issuerID == "" {
		return nil
	}
	return []string{"-authenticationKeyPath", keyPath, "-authenticationKeyID", keyID, "-authenticationKeyIssuerID", issuerID}
}

func submitCmd(cfg Config, args []string) error {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	version := fs.String("version", "", "App Store version")
	bundleID := fs.String("bundle-id", cfg.BundleID, "Bundle ID")
	buildNumber := fs.String("build-number", "", "App Store Connect build number")
	whatsNewPath := fs.String("whats-new", "", "What's New text file")
	noSubmit := fs.Bool("no-submit", false, "Prepare but do not submit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" {
		return fmt.Errorf("missing --version")
	}
	if *bundleID == "" {
		return fmt.Errorf("missing bundle id; set bundleId in .applship.json or pass --bundle-id")
	}
	whatsNew := "Bug fixes and improvements."
	if *whatsNewPath != "" {
		b, err := os.ReadFile(*whatsNewPath)
		if err != nil {
			return err
		}
		whatsNew = strings.TrimSpace(string(b))
	}
	client, err := NewASCClientFromEnv()
	if err != nil {
		return err
	}
	appID, err := client.FindAppID(*bundleID)
	if err != nil {
		return err
	}
	versionID, err := client.GetOrCreateVersion(appID, *version)
	if err != nil {
		return err
	}
	buildID, err := client.LatestEligibleBuild(appID, *buildNumber)
	if err != nil {
		return err
	}
	if err := client.AttachBuild(versionID, buildID); err != nil {
		return err
	}
	if err := client.UpdateWhatsNew(versionID, whatsNew); err != nil {
		return err
	}
	if *noSubmit {
		fmt.Printf("prepared version=%s build=%s\n", versionID, buildID)
		return nil
	}
	state, err := client.Submit(appID, versionID)
	if err != nil {
		return err
	}
	fmt.Printf("submitted version=%s build=%s state=%s\n", versionID, buildID, state)
	return nil
}

func status(cfg Config, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	bundleID := fs.String("bundle-id", cfg.BundleID, "Bundle ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundleID == "" {
		return fmt.Errorf("missing bundle id; set bundleId in .applship.json")
	}
	client, err := NewASCClientFromEnv()
	if err != nil {
		return err
	}
	return client.PrintStatus(*bundleID)
}

func release(cfg Config, args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	version := fs.String("version", "", "Marketing version")
	buildNumber := fs.String("build", "", "Build number")
	whatsNewPath := fs.String("whats-new", "", "What's New text file")
	doSubmit := fs.Bool("submit", false, "Submit for review after upload")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" {
		return fmt.Errorf("missing --version")
	}
	archivePath := filepath.Join(cfg.OutputDir, cfg.Scheme+"-"+*version+".xcarchive")
	archiveArgs := []string{"--version", *version}
	if *buildNumber != "" {
		archiveArgs = append(archiveArgs, "--build", *buildNumber)
	}
	if err := archive(cfg, archiveArgs); err != nil {
		return err
	}
	if err := upload(cfg, []string{"--archive", archivePath}); err != nil {
		return err
	}
	if !*doSubmit {
		return nil
	}
	fmt.Fprintln(os.Stderr, "waiting 90s for App Store Connect build processing...")
	time.Sleep(90 * time.Second)
	submitArgs := []string{"--version", *version}
	if *buildNumber != "" {
		submitArgs = append(submitArgs, "--build-number", *buildNumber)
	}
	if *whatsNewPath != "" {
		submitArgs = append(submitArgs, "--whats-new", *whatsNewPath)
	}
	return submitCmd(cfg, submitArgs)
}
