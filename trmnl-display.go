package main

import (
	"encoding/json"
	"flag"
	"fmt"
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
}

// AppOptions holds command line options
type AppOptions struct {
	DarkMode bool
	Verbose  bool
	BaseURL  string
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
			fmt.Println("Dark mode enabled - 1-bit BMP images will be inverted")
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

	for {
		processNextImage(tmpDir, config, options)
	}
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
	darkMode := flag.Bool("d", false, "Enable dark mode (invert 1-bit BMP images)")
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

func processNextImage(tmpDir string, config Config, options AppOptions) {
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

	// Display the image
	err = displayImage(filePath, options)
	if err != nil {
		fmt.Printf("Error displaying image: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}

	// Set default refresh rate if not provided
	refreshRate := terminal.RefreshRate
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
	
	out:
	// Sleep for the refresh rate
	for i := 0; i < refreshRate; i++ {
	    time.Sleep(time.Second) // sleep one second at a time
	    if done == 1 {
	        break out
	    }
	}
}

func displayImage(imagePath string, options AppOptions) error {
//
// N.B (Larry Bank)
// This update can use one of 3 temperature/panel profiles
// and the 3 update modes for 1-bit content
// Please consider if this should have a counter and mimic the TRMNL-OG behavior
//
        var sb strings.Builder
        var sb2 strings.Builder

        sb.WriteString("file=")
        sb.WriteString(imagePath)

        sb2.WriteString("invert=")
        if options.DarkMode {
              sb2.WriteString("true")
        } else {
              sb2.WriteString("false")
        }

        err := exec.Command("show_img", sb.String(), sb2.String(), "mode=fast").Run()
        if err != nil {
		fmt.Println("show_img tool missing; build it and try again; error = %v", err)
		os.Exit(0);
        }
	if options.Verbose {
		fmt.Printf("Displayed: %s\n", imagePath)
		fmt.Println("EPD update completed")
	}
	return nil
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

