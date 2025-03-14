package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
)

type GoVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// progressReader is a custom io.Reader that tracks download progress
type progressReader struct {
	reader         io.Reader
	totalBytes     int64
	readBytes      int64
	lastPercentage int
	lastUpdateTime time.Time
}

func newProgressReader(reader io.Reader, totalBytes int64) *progressReader {
	return &progressReader{
		reader:         reader,
		totalBytes:     totalBytes,
		lastUpdateTime: time.Now(),
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.readBytes += int64(n)

	// Update progress every 100ms to avoid too many updates
	if time.Since(pr.lastUpdateTime) > 100*time.Millisecond {
		percentage := int(float64(pr.readBytes) / float64(pr.totalBytes) * 100)

		// Only update if percentage changed
		if percentage != pr.lastPercentage && percentage <= 100 {
			pr.lastPercentage = percentage
			pr.lastUpdateTime = time.Now()

			// Create progress bar (Windows-compatible approach)
			progressBar := renderProgressBar(percentage)

			// Print the progress bar
			fmt.Print(progressBar)
		}
	}

	return n, err
}

// renderProgressBar creates a progress bar string that works on all platforms
func renderProgressBar(percentage int) string {
	// Ensure percentage is within bounds
	if percentage < 0 {
		percentage = 0
	} else if percentage > 100 {
		percentage = 100
	}

	// Calculate the width of the progress bar (50 characters)
	width := 50
	completed := width * percentage / 100

	// Build the progress bar
	var sb strings.Builder

	// Use carriage return to return to beginning of line
	sb.WriteString("\r")

	// Write the progress bar
	sb.WriteString("Downloading: [")
	sb.WriteString(strings.Repeat("=", completed))
	if completed < width {
		sb.WriteString(strings.Repeat(" ", width-completed))
	}
	sb.WriteString("] ")

	// Write the percentage
	sb.WriteString(fmt.Sprintf("%3d%%", percentage))

	return sb.String()
}

// printUsage prints the usage information for the getgo command
func printUsage() {
	bold := color.New(color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Printf("%s: getgo [options] [version] [install_path]\n", bold("Usage"))
	fmt.Printf("%s:\n", bold("Examples"))
	fmt.Printf("  %s                  # Latest version in current directory\n", cyan("getgo"))
	fmt.Printf("  %s                # Latest version in current directory\n", cyan("getgo -"))
	fmt.Printf("  %s           # Latest version in current directory\n", cyan("getgo latest"))
	fmt.Printf("  %s           # Specific version in current directory\n", cyan("getgo 1.23.1"))
	fmt.Printf("  %s     # Latest version in ~/.go\n", cyan("getgo latest ~/.go"))
	fmt.Printf("  %s  # Specific version in /usr/local/go\n", cyan("getgo 1.23.1 /usr/local/go"))
	fmt.Printf("  %s # Custom GOPATH\n", cyan("getgo --path ~/custom/gopath"))

	fmt.Printf("\n%s:\n", bold("Options"))
	fmt.Printf("  -h, --help         Show this help message\n")
	fmt.Printf("  -u, --unattended   Automatically set up environment variables (default: disabled)\n")
	fmt.Printf("  -p, --path PATH    Set custom GOPATH (default is $HOME/go)\n")
	fmt.Printf("  --envrc PATH       Create a .envrc file with Go environment variables at the specified path\n")
}

func main() {
	// Define flags
	helpFlag := flag.Bool("help", false, "Show usage information")
	hFlag := flag.Bool("h", false, "Show usage information")
	unattendedFlag := flag.Bool("unattended", false, "Automatically set up environment variables")
	uFlag := flag.Bool("u", false, "Automatically set up environment variables (shorthand)")
	gopathFlag := flag.String("path", "", "Custom GOPATH (default is $HOME/go)")
	gopathShortFlag := flag.String("p", "", "Custom GOPATH (shorthand)")
	envrcFlag := flag.String("envrc", "", "Path to add .envrc file with Go environment variables")

	flag.Parse()
	args := flag.Args()

	// Check if help was requested
	if isHelpRequested(helpFlag, hFlag) {
		printUsage()
		os.Exit(0)
	}

	// Default values
	versionArg := "latest"
	installPath := "." // Default to current directory

	// Parse arguments based on how many are provided
	switch len(args) {
	case 0:
		// Use defaults (latest version, current directory)
	case 1:
		versionArg = args[0]
	case 2:
		versionArg = args[0]
		installPath = args[1]
	default:
		printUsage()
		os.Exit(1)
	}

	// Expand and convert installPath to absolute path
	installPath = expandPathOrExit(installPath)

	// Get the version to download
	version := versionArg
	if version == "latest" || version == "-" {
		var err error
		color.Cyan("Fetching latest Go version...")
		version, err = getLatestGoVersion()
		if err != nil {
			color.Red("Error getting latest Go version: %v", err)
			os.Exit(1)
		}
		color.Green("Latest Go version is %s", version)
	}

	// Create the download URL
	osName := runtime.GOOS
	arch := runtime.GOARCH

	var archiveExt string
	if osName == "windows" {
		archiveExt = "zip"
	} else {
		archiveExt = "tar.gz"
	}

	downloadURL := fmt.Sprintf("https://go.dev/dl/go%s.%s-%s.%s", version, osName, arch, archiveExt)

	// Check if the version already exists at the destination
	versionedGoDir := filepath.Join(installPath, fmt.Sprintf("go%s", version))

	// Ensure versionedGoDir is absolute (should already be since installPath is absolute)
	versionedGoDir = expandPathOrExit(versionedGoDir)

	// Get the user's home directory for GOPATH
	usr, err := user.Current()
	if err != nil {
		color.Red("Error getting current user: %v", err)
		os.Exit(1)
	}

	// Set GOPATH - use custom path if provided, otherwise default to $HOME/go
	gopath := filepath.Join(usr.HomeDir, "go")

	// Check if a custom GOPATH was provided via flags
	customPath := getCustomGOPATH(gopathFlag, gopathShortFlag)

	if customPath != "" {
		// Expand and convert custom GOPATH to absolute path
		customPath = expandPathOrExit(customPath)

		gopath = customPath
		//color.Cyan("Using custom GOPATH: %s", gopath)
	}

	if _, err := os.Stat(versionedGoDir); err == nil {
		color.Yellow("Go version %s already exists at %s", version, versionedGoDir)

		// Print environment variables
		printEnvVars(versionedGoDir, gopath)

		// Set up environment variables if requested
		if isUnattendedMode(unattendedFlag, uFlag) {
			setupEnvironmentVariables(versionedGoDir, gopath)
		}

		// Set up .envrc file if requested
		setupEnvrcIfRequested(envrcFlag, versionedGoDir, gopath)

		os.Exit(0)
	}

	// Create the installation directory if it doesn't exist
	err = os.MkdirAll(installPath, 0755)
	if err != nil {
		color.Red("Error creating installation directory: %v", err)
		os.Exit(1)
	}

	// Download the Go archive
	color.Cyan("Downloading Go %s for %s/%s...", version, osName, arch)
	archivePath := filepath.Join(os.TempDir(), fmt.Sprintf("go%s.%s-%s.%s", version, osName, arch, archiveExt))
	err = downloadFileWithProgress(downloadURL, archivePath)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			color.Red("Error: Go version %s not found for %s/%s", version, osName, arch)
			fmt.Println("Please check that the version exists at https://go.dev/dl/")
		} else {
			color.Red("Error downloading Go archive: %v", err)
		}
		os.Exit(1)
	}
	fmt.Println() // Add a newline after progress bar

	// Extract the archive
	color.Cyan("Extracting to %s ...", installPath)

	// Create a temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "getgo-extract")
	if err != nil {
		color.Red("Error creating temporary directory: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	if osName == "windows" {
		err = unzip(archivePath, tempDir)
	} else {
		err = untargz(archivePath, tempDir)
	}
	if err != nil {
		color.Red("Error extracting archive: %v", err)
		os.Exit(1)
	}

	// Move the extracted "go" directory to the versioned directory
	extractedGoDir := filepath.Join(tempDir, "go")
	versionedGoDir = filepath.Join(installPath, fmt.Sprintf("go%s", version))

	// Ensure versionedGoDir is absolute (should already be since installPath is absolute)
	versionedGoDir = expandPathOrExit(versionedGoDir)

	// Remove the destination directory if it already exists
	if _, err := os.Stat(versionedGoDir); err == nil {
		if err := os.RemoveAll(versionedGoDir); err != nil {
			color.Red("Error removing existing directory: %v", err)
			os.Exit(1)
		}
	}

	// Create the parent directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(versionedGoDir), 0755); err != nil {
		color.Red("Error creating parent directory: %v", err)
		os.Exit(1)
	}

	// Rename the extracted directory to the versioned directory
	if err := os.Rename(extractedGoDir, versionedGoDir); err != nil {
		color.Red("Error moving extracted directory: %v", err)
		os.Exit(1)
	}

	// Clean up the downloaded archive
	os.Remove(archivePath)

	color.Green("Go %s has been successfully installed to %s", version, versionedGoDir)

	// Print environment variables
	printEnvVars(versionedGoDir, gopath)

	// Set up environment variables if requested
	if isUnattendedMode(unattendedFlag, uFlag) {
		setupEnvironmentVariables(versionedGoDir, gopath)
	}

	// Set up .envrc file if requested
	setupEnvrcIfRequested(envrcFlag, versionedGoDir, gopath)
}

// setupEnvironmentVariables sets up environment variables in the appropriate configuration files
func setupEnvironmentVariables(goroot, gopath string) {
	if runtime.GOOS == "windows" {
		setupWindowsEnvironment(goroot, gopath)
	} else {
		setupUnixEnvironment(goroot, gopath)
	}
}

// setupUnixEnvironment sets up environment variables in Unix-like systems (Linux, macOS)
func setupUnixEnvironment(goroot, gopath string) {
	// Determine the shell configuration file
	shellConfigFile := getShellConfigFile()
	if shellConfigFile == "" {
		color.Yellow("Could not determine shell configuration file. Please set up environment variables manually.")
		return
	}

	// Create environment variable exports
	exports := []string{
		fmt.Sprintf("export GOROOT=%s", goroot),
		fmt.Sprintf("export GOPATH=%s", gopath),
		fmt.Sprintf("export PATH=$PATH:$GOPATH/bin:$GOROOT/bin"),
	}

	// Check if the file exists
	if _, err := os.Stat(shellConfigFile); os.IsNotExist(err) {
		color.Yellow("Shell configuration file %s does not exist. Creating it...", shellConfigFile)
		os.Create(shellConfigFile)
	}

	// Read the current content of the file
	content, err := os.ReadFile(shellConfigFile)
	if err != nil {
		color.Red("Error reading shell configuration file: %v", err)
		return
	}

	// Check if Go environment variables are already set
	if strings.Contains(string(content), "GOROOT=") {
		color.Yellow("Go environment variables already exist in %s", shellConfigFile)
		color.Yellow("You may need to update them manually:")
		for _, export := range exports {
			fmt.Println(export)
		}
		return
	}

	// Append the exports to the file
	f, err := os.OpenFile(shellConfigFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		color.Red("Error opening shell configuration file: %v", err)
		return
	}
	defer f.Close()

	// Write a comment and the exports
	_, err = f.WriteString("\n# Go environment variables added by getgo\n")
	if err != nil {
		color.Red("Error writing to shell configuration file: %v", err)
		return
	}

	for _, export := range exports {
		_, err = f.WriteString(export + "\n")
		if err != nil {
			color.Red("Error writing to shell configuration file: %v", err)
			return
		}
	}

	color.Green("Go environment variables have been added to %s", shellConfigFile)
	color.Yellow("Run 'source %s' to apply the changes to your current shell", shellConfigFile)
}

// setupWindowsEnvironment sets up environment variables in Windows
func setupWindowsEnvironment(goroot, gopath string) {
	// Use PowerShell to set environment variables
	color.Cyan("Setting up environment variables using PowerShell...")

	// Set GOROOT
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("[Environment]::SetEnvironmentVariable('GOROOT', '%s', 'User')", goroot))
	err := cmd.Run()
	if err != nil {
		color.Red("Error setting GOROOT: %v", err)
		return
	}

	// Set GOPATH
	cmd = exec.Command("powershell", "-Command",
		fmt.Sprintf("[Environment]::SetEnvironmentVariable('GOPATH', '%s', 'User')", gopath))
	err = cmd.Run()
	if err != nil {
		color.Red("Error setting GOPATH: %v", err)
		return
	}

	// Update PATH
	cmd = exec.Command("powershell", "-Command", `
		$currentPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
		$goPathBin = Join-Path -Path $env:GOPATH -ChildPath 'bin'
		$goRootBin = Join-Path -Path $env:GOROOT -ChildPath 'bin'
		if (-not $currentPath.Contains($goPathBin) -and -not $currentPath.Contains($goRootBin)) {
			$newPath = $currentPath + ';' + $goPathBin + ';' + $goRootBin
			[Environment]::SetEnvironmentVariable('PATH', $newPath, 'User')
		}
	`)
	err = cmd.Run()
	if err != nil {
		color.Red("Error updating PATH: %v", err)
		return
	}

	color.Green("Go environment variables have been set up successfully")
	color.Yellow("Please restart your terminal or system for the changes to take effect")
}

// getShellConfigFile determines the appropriate shell configuration file
func getShellConfigFile() string {
	// Get the current shell
	shell := os.Getenv("SHELL")

	// Get the user's home directory
	usr, err := user.Current()
	if err != nil {
		return ""
	}

	// Determine the configuration file based on the shell
	switch {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(usr.HomeDir, ".zshrc")
	case strings.Contains(shell, "bash"):
		// Check for .bash_profile first on macOS
		if runtime.GOOS == "darwin" {
			bashProfile := filepath.Join(usr.HomeDir, ".bash_profile")
			if _, err := os.Stat(bashProfile); err == nil {
				return bashProfile
			}
		}
		return filepath.Join(usr.HomeDir, ".bashrc")
	case strings.Contains(shell, "fish"):
		fishConfig := filepath.Join(usr.HomeDir, ".config", "fish", "config.fish")
		// Create the directory if it doesn't exist
		os.MkdirAll(filepath.Dir(fishConfig), 0755)
		return fishConfig
	default:
		// Try to find a common shell configuration file
		for _, file := range []string{".profile", ".bashrc", ".bash_profile", ".zshrc"} {
			path := filepath.Join(usr.HomeDir, file)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	// Default to .profile if no other file is found
	return filepath.Join(usr.HomeDir, ".profile")
}

// printEnvVars prints the environment variables needed for Go based on the OS
func printEnvVars(goroot, gopath string) {
	bold := color.New(color.Bold).SprintFunc()

	if runtime.GOOS == "windows" {
		fmt.Printf("\n%s:\n\n", bold("Go environment variables"))
		fmt.Printf("GOROOT=%s\n", goroot)
		fmt.Printf("GOPATH=%s\n", gopath)
		fmt.Printf("PATH=%%PATH%%;%%GOPATH%%\\bin;%%GOROOT%%\\bin\n")
	} else {
		fmt.Printf("\n%s:\n\n", bold("Go environment variables"))
		fmt.Printf("export GOROOT=%s\n", goroot)
		fmt.Printf("export GOPATH=%s\n", gopath)
		fmt.Printf("export PATH=$PATH:$GOPATH/bin:$GOROOT/bin\n")
	}
	fmt.Println()
}

func getLatestGoVersion() (string, error) {
	resp, err := http.Get("https://go.dev/dl/?mode=json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var versions []GoVersion
	err = json.NewDecoder(resp.Body).Decode(&versions)
	if err != nil {
		return "", err
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no Go versions found")
	}

	// Find the first stable version
	for _, v := range versions {
		if v.Stable {
			// Remove the "go" prefix from the version
			return strings.TrimPrefix(v.Version, "go"), nil
		}
	}

	// If no stable version is found, return the first version
	return strings.TrimPrefix(versions[0].Version, "go"), nil
}

func downloadFileWithProgress(url, filepath string) error {
	// Send HEAD request to get the file size
	headResp, err := http.Head(url)
	if err != nil {
		return err
	}
	defer headResp.Body.Close()

	if headResp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s (URL: %s)", headResp.Status, url)
	}

	totalBytes := headResp.ContentLength

	// Now send the actual GET request
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s (URL: %s)", resp.Status, url)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Create a progress reader
	progressR := newProgressReader(resp.Body, totalBytes)

	// Copy the data using the progress reader
	_, err = io.Copy(out, progressR)

	// Ensure the progress bar shows 100% when download is complete
	fmt.Print(renderProgressBar(100))

	return err
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s (URL: %s)", resp.Status, url)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func untargz(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dst, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			outFile, err := os.Create(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
			if err := os.Chmod(path, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}
	return nil
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(dst, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

// expandPath expands a path with ~ and converts it to an absolute path
func expandPath(path string) (string, error) {
	// Expand ~ in the path
	if path == "~" || strings.HasPrefix(path, "~/") {
		usr, err := user.Current()
		if err != nil {
			return "", fmt.Errorf("error getting current user: %v", err)
		}

		if path == "~" {
			path = usr.HomeDir
		} else {
			path = filepath.Join(usr.HomeDir, path[2:])
		}
	}

	// Convert to absolute path if it's not already
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("error resolving absolute path: %v", err)
		}
		path = absPath
	}

	return path, nil
}

// isUnattendedMode checks if unattended mode is enabled
func isUnattendedMode(unattendedFlag, uFlag *bool) bool {
	return *unattendedFlag || *uFlag
}

// isHelpRequested checks if help was requested
func isHelpRequested(helpFlag, hFlag *bool) bool {
	return *helpFlag || *hFlag
}

// getCustomGOPATH returns the custom GOPATH from flags if provided
func getCustomGOPATH(gopathFlag, gopathShortFlag *string) string {
	customPath := *gopathFlag
	if customPath == "" {
		customPath = *gopathShortFlag
	}
	return customPath
}

// expandPathOrExit expands a path and exits on error
func expandPathOrExit(path string) string {
	expandedPath, err := expandPath(path)
	if err != nil {
		color.Red("%v", err)
		os.Exit(1)
	}
	return expandedPath
}

// setupEnvrcIfRequested sets up a .envrc file if the envrcFlag is provided
func setupEnvrcIfRequested(envrcFlag *string, goroot, gopath string) {
	if *envrcFlag != "" {
		err := setupEnvrcFile(*envrcFlag, goroot, gopath)
		if err != nil {
			color.Red("Error setting up .envrc file: %v", err)
		} else {
			color.Yellow("Run 'direnv allow' to enable the environment variables")
		}
	}
}

// setupEnvrcFile creates or updates a .envrc file with Go environment variables
func setupEnvrcFile(envrcPath, goroot, gopath string) error {
	// Expand the path if needed
	expandedPath, err := expandPath(envrcPath)
	if err != nil {
		return fmt.Errorf("error expanding envrc path: %v", err)
	}

	// If the path is a directory, append .envrc to it
	fileInfo, err := os.Stat(expandedPath)
	if err == nil && fileInfo.IsDir() {
		expandedPath = filepath.Join(expandedPath, ".envrc")
	}

	// Create the directory if it doesn't exist
	err = os.MkdirAll(filepath.Dir(expandedPath), 0755)
	if err != nil {
		return fmt.Errorf("error creating directory for .envrc: %v", err)
	}

	// Check if the file already exists
	fileExists := false
	if _, err := os.Stat(expandedPath); err == nil {
		fileExists = true

		// Check if Go environment variables are already set in the file
		content, err := os.ReadFile(expandedPath)
		if err != nil {
			return fmt.Errorf("error reading existing .envrc file: %v", err)
		}

		if strings.Contains(string(content), "GOROOT=") {
			color.Yellow("Go environment variables already exist in %s", expandedPath)
			color.Yellow("Not modifying the existing .envrc file")
			return nil
		}
	}

	// Open the file in append mode if it exists, or create it if it doesn't
	var f *os.File
	if fileExists {
		f, err = os.OpenFile(expandedPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error opening .envrc file: %v", err)
		}

		// Add a newline before our content if the file doesn't end with one
		content, err := os.ReadFile(expandedPath)
		if err != nil {
			f.Close()
			return fmt.Errorf("error reading .envrc file: %v", err)
		}

		if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
			_, err = f.WriteString("\n")
			if err != nil {
				f.Close()
				return fmt.Errorf("error writing to .envrc file: %v", err)
			}
		}
	} else {
		f, err = os.OpenFile(expandedPath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error creating .envrc file: %v", err)
		}
	}
	defer f.Close()

	// Write the environment variables
	_, err = f.WriteString("\n# Go environment variables added by getgo\n")
	if err != nil {
		return fmt.Errorf("error writing to .envrc file: %v", err)
	}

	// Write the exports based on the OS
	if runtime.GOOS == "windows" {
		_, err = f.WriteString(fmt.Sprintf("export GOROOT=\"%s\"\n", goroot))
		if err != nil {
			return fmt.Errorf("error writing to .envrc file: %v", err)
		}

		_, err = f.WriteString(fmt.Sprintf("export GOPATH=\"%s\"\n", gopath))
		if err != nil {
			return fmt.Errorf("error writing to .envrc file: %v", err)
		}

		_, err = f.WriteString("export PATH=\"$PATH:$GOPATH/bin:$GOROOT/bin\"\n")
		if err != nil {
			return fmt.Errorf("error writing to .envrc file: %v", err)
		}
	} else {
		_, err = f.WriteString(fmt.Sprintf("export GOROOT=%s\n", goroot))
		if err != nil {
			return fmt.Errorf("error writing to .envrc file: %v", err)
		}

		_, err = f.WriteString(fmt.Sprintf("export GOPATH=%s\n", gopath))
		if err != nil {
			return fmt.Errorf("error writing to .envrc file: %v", err)
		}

		_, err = f.WriteString("export PATH=$PATH:$GOPATH/bin:$GOROOT/bin\n")
		if err != nil {
			return fmt.Errorf("error writing to .envrc file: %v", err)
		}
	}

	if fileExists {
		color.Green("Appended Go environment variables to existing .envrc file at %s", expandedPath)
	} else {
		color.Green("Created new .envrc file with Go environment variables at %s", expandedPath)
	}

	return nil
}
