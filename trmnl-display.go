package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"bufio"

	// Register the decoders for the image formats a TRMNL server may serve so
	// image.Decode can turn the downloaded file into pixels for comparison.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Version information
var (
	version   = "0.1.1"
	commit    = "unknown"
	buildDate = "unknown"
)

// TerminalResponse represents the JSON structure returned by the API
type TerminalResponse struct {
	ImageURL    string `json:"image_url"`
	Filename    string `json:"filename"`
	RefreshRate int    `json:"refresh_rate"`
}

// Config holds application configuration
type Config struct {
	APIKey   string `json:"api_key,omitempty"`   // API key for trmnl.app
	DeviceID string `json:"device_id,omitempty"` // Device ID (MAC address) for Terminus/BYOS servers
	BaseURL  string `json:"base_url,omitempty"`
	// DisplayCommand is the external command used to render an image on the
	// display. Defaults to "show_img" (the bb_epaper backend). build.sh sets
	// this to the inky Python renderer when an Inky board is selected. The
	// value may contain arguments (e.g. "/path/python3 /path/inky_display.py");
	// the image/invert/mode arguments are appended when it is invoked.
	DisplayCommand string `json:"display_command,omitempty"`
}

// AppOptions holds command line options
type AppOptions struct {
	DarkMode bool
	Verbose  bool
	BaseURL  string
	// DisplayCommand mirrors Config.DisplayCommand; resolved in main().
	DisplayCommand string
}

//  exec.Command("sudo", "service", "gpm", "stop").Run()

func main() {
	// Parse command line arguments
	options := parseCommandLineArgs()

	// Set up signal handling for clean exit
	setupSignalHandling()

	// Check the environment first
	if options.Verbose {
		fmt.Println("Checking system environment...")
		if options.DarkMode {
			fmt.Println("Dark mode enabled - images will be inverted")
		}
	}

	var err error

	// Create a configuration directory as per XDG standard:
	// at user-specified location when the environment variable is set,
	// at $HOME/.config/trmnl (XDG default config location for Unix) if not set
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		homeDir, err := os.UserHomeDir()
        	if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			os.Exit(1)
		}
        	configHome = filepath.Join(homeDir, ".config")
    	}
	configDir := filepath.Join(configHome, "trmnl")
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		fmt.Printf("Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	// Get configuration from file
	config := loadConfig(configDir)

	// Override with environment variables if present
	if envAPIKey := os.Getenv("TRMNL_API_KEY"); envAPIKey != "" {
		config.APIKey = envAPIKey
	}
	if envDeviceID := os.Getenv("TRMNL_DEVICE_ID"); envDeviceID != "" {
		config.DeviceID = envDeviceID
	}
	if envBaseURL := os.Getenv("TRMNL_BASE_URL"); envBaseURL != "" {
		config.BaseURL = envBaseURL
	}

	// Override with command line argument if provided
	if options.BaseURL != "" {
		config.BaseURL = options.BaseURL
	}

	// Set default base URL if not configured
	if config.BaseURL == "" {
		config.BaseURL = "https://trmnl.app"
	}

	// Resolve the display backend command (see Config.DisplayCommand).
	options.DisplayCommand = config.DisplayCommand

	if options.Verbose {
		fmt.Printf("Using base URL: %s\n", config.BaseURL)
	}

	// Check if we're using trmnl.app or a custom server
	isTerminusServer := !strings.Contains(config.BaseURL, "trmnl.app")

	// Ensure we have the appropriate credentials
	if isTerminusServer {
		// For Terminus/BYOS servers, we need a device ID (MAC address)
		if config.DeviceID == "" {
			// Check if API key looks like a MAC address and migrate it
			if config.APIKey != "" && strings.Count(config.APIKey, ":") == 5 {
				config.DeviceID = config.APIKey
				config.APIKey = "" // Clear API key since it's actually a device ID
			} else {
				fmt.Println("Device ID (MAC address) not found.")
				fmt.Print("Please enter your device MAC address (e.g., AA:BB:CC:DD:EE:FF): ")
				fmt.Scanln(&config.DeviceID)
			}
			saveConfig(configDir, config)
		}
	} else {
		// For trmnl.app, we need an API key
		if config.APIKey == "" {
			fmt.Println("TRMNL (device) API Key not found.")
                        fmt.Println("(in the Device Credentials section of the web portal)")
			fmt.Print("Please enter your key: ")
			fmt.Scanln(&config.APIKey)
			saveConfig(configDir, config)
		}
	}

	// Create a temporary directory for storing images
	tmpDir, err := os.MkdirTemp("", "trmnl-display")
	if err != nil {
		fmt.Printf("Error creating temp directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)
	state := &displayState{}
	for {
		processNextImage(tmpDir, config, options, state)
	}
}

// displayState carries the bits of state that must persist between refreshes:
// the pixel hash of the image currently on the panel (so we can skip redundant
// updates) and the count of actual updates performed (which drives the
// periodic ghost-clearing full refresh).
type displayState struct {
	LastHash string
	Frames   int
}

// setupSignalHandling sets up handlers for SIGINT, SIGTERM, and SIGHUP
func setupSignalHandling() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-c
		fmt.Println("\nReceived termination signal. Cleaning up...")
		os.Exit(0)
	}()
}

// parseCommandLineArgs parses command line arguments and returns app options
func parseCommandLineArgs() AppOptions {
	darkMode := flag.Bool("d", false, "Enable dark mode (invert image pixels)")
	showVersion := flag.Bool("v", false, "Show version information")
	verbose := flag.Bool("verbose", true, "Enable verbose output")
	quiet := flag.Bool("q", false, "Quiet mode (disable verbose output)")
	baseURL := flag.String("base-url", "", "Custom base URL for the TRMNL API (default: https://trmnl.app)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("trmnl-display version %s (commit: %s, built: %s)\n",
			version, commit, buildDate)
		os.Exit(0)
	}

	return AppOptions{
		DarkMode: *darkMode,
		Verbose:  *verbose && !*quiet,
		BaseURL:  *baseURL,
	}
}

func processNextImage(tmpDir string, config Config, options AppOptions, state *displayState) {
	// Use defer and recover to handle any panics
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic: %v\n", r)
			time.Sleep(60 * time.Second)
		}
	}()

	// Get the TRMNL display
	apiURL := strings.TrimRight(config.BaseURL, "/") + "/api/display"
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}

	// Use different header based on server type
	// For Terminus servers, use MAC address in ID header
	// For standard TRMNL servers, use access-token
	if strings.Contains(config.BaseURL, "trmnl.app") {
		req.Header.Add("access-token", config.APIKey)
	} else {
		// For Terminus/BYOS servers, use ID header with MAC address
		req.Header.Add("ID", config.DeviceID)
		// Also add access-token for BYOS Laravel compatibility
		if config.APIKey != "" {
			req.Header.Add("access-token", config.APIKey)
		}
		req.Header.Add("Content-Type", "application/json")
	}
	req.Header.Add("battery-voltage", "100.00")
	req.Header.Add("rssi", "0")
	req.Header.Add("User-Agent", fmt.Sprintf("trmnl-display/%s", version))
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error fetching display: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("Error fetching display from %s: status code %d\n", apiURL, resp.StatusCode)
		if options.Verbose && resp.StatusCode == 404 {
			fmt.Printf("API endpoint not found. Please verify the base URL is correct.\n")
		}
		time.Sleep(60 * time.Second)
		return
	}

	// Parse the JSON response
	var terminal TerminalResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&terminal); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}

	// Set default filename if not provided
	filename := terminal.Filename
	if filename == "" {
		filename = "display.jpg"
	}

	// Create full path to temporary file
	filePath := filepath.Join(tmpDir, filename)

	// Download the image
	imgResp, err := http.Get(terminal.ImageURL)
	if err != nil {
		fmt.Printf("Error downloading image: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}
	defer imgResp.Body.Close()

	// Create the file
	out, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}

	// Copy the image data to the file
	_, err = io.Copy(out, imgResp.Body)
	if err != nil {
		fmt.Printf("Error saving image: %v\n", err)
		out.Close()
		time.Sleep(60 * time.Second)
		return
	}
	out.Close()

	// Skip the panel update when the new image is pixel-identical to the one
	// already on screen. The server re-encodes the PNG on every poll so the
	// file bytes differ even when the content does not; comparing decoded
	// pixels (via a hash) is what actually tells us whether anything changed.
	// If the image can't be decoded we fall through and display it, so an
	// unsupported format never causes us to wrongly skip an update.
	newHash, hashErr := hashImagePixels(filePath)
	if hashErr != nil {
		if options.Verbose {
			fmt.Printf("Could not decode image for change detection (%v); displaying anyway\n", hashErr)
		}
	} else if newHash == state.LastHash {
		if options.Verbose {
			fmt.Println("Image unchanged since last update; skipping panel refresh")
		}
		waitForRefresh(terminal.RefreshRate)
		return
	}

	// Display the image
	err = displayImage(filePath, options, state.Frames)
	if err != nil {
		fmt.Printf("Error displaying image: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}
	// Only record the hash and advance the update counter once the panel has
	// actually been refreshed, so the periodic ghost-clearing full refresh
	// counts real updates rather than skipped polls.
	state.LastHash = newHash
	state.Frames++

	waitForRefresh(terminal.RefreshRate)
}

// waitForRefresh sleeps until the next poll is due, waking early if the user
// presses a key. A non-positive rate falls back to 60 seconds.
func waitForRefresh(refreshRate int) {
	if refreshRate <= 0 {
		refreshRate = 60
	}

	done := 0

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			fmt.Println("Keypress...skipping to next update")
			done = 1
			break
		}
	}()

	// Sleep for the refresh rate, one second at a time so a keypress takes
	// effect promptly.
	for i := 0; i < refreshRate; i++ {
		time.Sleep(time.Second)
		if done == 1 {
			return
		}
	}
}

func displayImage(imagePath string, options AppOptions, frames int) error {
//
// N.B (Larry Bank)
// This update can use one of 3 temperature/panel profiles
// and the 3 update modes for 1-bit content
// Please consider if this should have a counter and mimic the TRMNL-OG behavior
//
        var sb strings.Builder
        var sb2 strings.Builder
        var sb3 strings.Builder

        sb.WriteString("file=")
        sb.WriteString(imagePath)

        sb2.WriteString("invert=")
        if options.DarkMode {
              sb2.WriteString("true")
        } else {
              sb2.WriteString("false")
        }

        sb3.WriteString("mode=")
        if (frames & 3) == 0 { // use fast mode every 4 updates to clear any ghosting
              sb3.WriteString("fast")
        } else {
              sb3.WriteString("partial") // partial = no flicker/flash
        }
        // Resolve the render backend. Defaults to the bb_epaper "show_img"
        // binary; an Inky board uses the inky Python renderer instead. The
        // command may include its own arguments (e.g. "python3 inky_display.py"),
        // so split it and append the file/invert/mode arguments.
        displayCommand := options.DisplayCommand
        if displayCommand == "" {
              displayCommand = "show_img"
        }
        fields := strings.Fields(displayCommand)
        cmdName := fields[0]
        cmdArgs := append([]string{}, fields[1:]...)
        cmdArgs = append(cmdArgs, sb.String(), sb2.String(), sb3.String())
        err := exec.Command(cmdName, cmdArgs...).Run()
        if err != nil {
		fmt.Printf("%s render command failed; check it is installed/built; error = %v\n", cmdName, err)
		os.Exit(0);
        }
	if options.Verbose {
		fmt.Printf("Displayed: %s\n", imagePath)
		fmt.Println("EPD update completed")
	}
	return nil
}

// hashImagePixels decodes the image at path and returns a hash of its raw
// pixels. Two files with identical visual content hash the same even if their
// encoded bytes differ (e.g. the server re-compresses the PNG on every poll),
// which lets the caller detect that nothing has actually changed. The image is
// normalized to RGBA first so the comparison is independent of the decoded
// colour model, and the dimensions are folded in so differently sized images
// can never collide.
func hashImagePixels(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	h := sha256.New()
	fmt.Fprintf(h, "%dx%d:", bounds.Dx(), bounds.Dy())
	h.Write(rgba.Pix)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func loadConfig(configDir string) Config {
	configFile := filepath.Join(configDir, "config.json")
	config := Config{}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return config
	}

	_ = json.Unmarshal(data, &config)
	return config
}

func saveConfig(configDir string, config Config) {
	configFile := filepath.Join(configDir, "config.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		return
	}

	err = os.WriteFile(configFile, data, 0600)
	if err != nil {
		fmt.Printf("Error writing config file: %v\n", err)
	}
}
