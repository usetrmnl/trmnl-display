package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	_ "golang.org/x/image/bmp" // Register BMP decoder

	imagedraw "golang.org/x/image/draw"

	"github.com/gonutz/framebuffer"
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
	APIKey string
}

// AppOptions holds command line options
type AppOptions struct {
	DarkMode bool
	Verbose  bool
}

// FramebufferLock represents the lock file structure
type FramebufferLock struct {
	LockPath string
	Acquired bool
}

// Global lock variable for cleanup
var fbLock *FramebufferLock

// Add this new function to disable the cursor
func disableCursor() error {
	// Method 1: Using the terminal settings
	termios := syscall.Termios{
		Iflag: 0,
		Oflag: 0,
		Cflag: 0,
		Lflag: 0,
	}

	tty, err := os.OpenFile("/dev/tty1", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("error opening /dev/tty1: %v", err)
	}
	defer tty.Close()

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return fmt.Errorf("ioctl error: %v", errno)
	}

	// Method 2: Try to disable the cursor via escape sequence
	_, err = tty.Write([]byte("\033[?25l"))
	if err != nil {
		return fmt.Errorf("error writing escape sequence: %v", err)
	}

	// Method 3: Use the console blinking cursor control
	err = ioutil.WriteFile("/sys/class/graphics/fbcon/cursor_blink", []byte("0"), 0644)
	if err != nil {
		fmt.Printf("Warning: Failed to disable cursor blink via sysfs: %v\n", err)
		// Not returning error as this is optional
	}

	// Method 4: Try to disable GPM mouse daemon if running
	if _, err := os.Stat("/var/run/gpm.pid"); err == nil {
		fmt.Println("GPM mouse daemon detected, attempting to disable it...")
		exec.Command("sudo", "service", "gpm", "stop").Run()
	}

	return nil
}

func restoreCursor() {
	tty, err := os.OpenFile("/dev/tty1", os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer tty.Close()

	// Re-enable cursor
	tty.Write([]byte("\033[?25h"))

	// Re-enable cursor blink
	ioutil.WriteFile("/sys/class/graphics/fbcon/cursor_blink", []byte("1"), 0644)
}

func main() {
	// Check root privileges
	checkRoot()

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
		checkDisplayServer()
		listFramebufferDevices()
	}

	// Create a configuration directory
	configDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	configDir = filepath.Join(configDir, ".trmnl")
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		fmt.Printf("Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	// Get API key from environment, or from config file
	config := loadConfig(configDir)
	if config.APIKey == "" {
		config.APIKey = os.Getenv("TRMNL_API_KEY")
	}

	// If the API key is still not set, prompt the user
	if config.APIKey == "" {
		fmt.Println("TRMNL API Key not found.")
		fmt.Print("Please enter your TRMNL API Key: ")
		fmt.Scanln(&config.APIKey)
		saveConfig(configDir, config)
	}

	// Create a temporary directory for storing images
	tmpDir, err := os.MkdirTemp("", "trmnl-display")
	if err != nil {
		fmt.Printf("Error creating temp directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	// Create and acquire framebuffer lock
	fbLock = NewFramebufferLock("/var/lock/trmnl-display.lock")
	err = fbLock.Acquire()
	if err != nil {
		fmt.Printf("Error acquiring framebuffer lock: %v\n", err)
		os.Exit(1)
	}
	defer fbLock.Release()

	// Disable cursor
	if err := disableCursor(); err != nil {
		fmt.Printf("Warning: Failed to disable cursor: %v\n", err)
		// Continue anyway, as this is not critical
	}

	// Clear the framebuffer at startup
	clearFramebuffer()

	for {
		processNextImage(tmpDir, config.APIKey, options)
	}
}

// NewFramebufferLock creates a new framebuffer lock
func NewFramebufferLock(lockPath string) *FramebufferLock {
	return &FramebufferLock{
		LockPath: lockPath,
		Acquired: false,
	}
}

// Acquire attempts to acquire the framebuffer lock
func (l *FramebufferLock) Acquire() error {
	// First check if the lock file exists
	if _, err := os.Stat(l.LockPath); err == nil {
		// Lock file exists, check if it's stale
		pid, err := l.readLockFile()
		if err != nil {
			return fmt.Errorf("error reading lock file: %v", err)
		}

		// Check if the process is still running
		if l.isProcessRunning(pid) {
			return fmt.Errorf("framebuffer is currently in use by process %d", pid)
		}

		// Lock is stale, remove it
		fmt.Printf("Removing stale lock from PID %d\n", pid)
		if err := os.Remove(l.LockPath); err != nil {
			return fmt.Errorf("error removing stale lock file: %v", err)
		}
	}

	// Create the lock file with current PID
	if err := l.writeLockFile(); err != nil {
		return fmt.Errorf("error creating lock file: %v", err)
	}

	l.Acquired = true
	fmt.Println("Acquired exclusive framebuffer access")
	return nil
}

// Release releases the framebuffer lock
func (l *FramebufferLock) Release() {
	if l.Acquired {
		if err := os.Remove(l.LockPath); err != nil {
			fmt.Printf("Error removing lock file: %v\n", err)
		} else {
			fmt.Println("Released framebuffer lock")
			l.Acquired = false
		}
	}
}

// readLockFile reads the PID from the lock file
func (l *FramebufferLock) readLockFile() (int, error) {
	data, err := os.ReadFile(l.LockPath)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in lock file: %v", err)
	}

	return pid, nil
}

// writeLockFile writes the current PID to the lock file
func (l *FramebufferLock) writeLockFile() error {
	pid := os.Getpid()
	return os.WriteFile(l.LockPath, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// isProcessRunning checks if a process with the given PID is running
func (l *FramebufferLock) isProcessRunning(pid int) bool {
	// Try to find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return false // Can't find process, must not be running
	}

	// On Unix, FindProcess always succeeds, so we need to send a signal
	// to check if the process actually exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// setupSignalHandling sets up handlers for SIGINT, SIGTERM, and SIGHUP
func setupSignalHandling() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-c
		fmt.Println("\nReceived termination signal. Cleaning up...")
		if fbLock != nil {
			fbLock.Release()
		}
		clearFramebuffer()
		restoreCursor() // Restore cursor before exiting
		os.Exit(0)
	}()
}

// clearFramebuffer fills the framebuffer with black to clear it
func clearFramebuffer() {
	fmt.Println("Clearing framebuffer...")

	fb, err := framebuffer.Open("/dev/fb0")
	if err != nil {
		fmt.Printf("Error opening framebuffer to clear: %v\n", err)
		return
	}
	defer fb.Close()

	// Create a black image
	black := image.NewRGBA(fb.Bounds())
	draw.Draw(fb, fb.Bounds(), black, image.Point{}, draw.Src)

	// Flush the framebuffer if necessary
	if fbFlusher, ok := interface{}(fb).(interface{ Flush() error }); ok {
		fbFlusher.Flush()
	}
}

// checkRoot verifies if the program is running with root privileges
func checkRoot() {
	currentUser, err := user.Current()
	if err != nil {
		fmt.Printf("Error determining current user: %v\n", err)
		os.Exit(1)
	}

	if currentUser.Uid != "0" {
		fmt.Println("This program requires root privileges to access the framebuffer.")
		fmt.Println("Please run with sudo or as root.")
		os.Exit(1)
	}

	fmt.Println("Running with root privileges ✓")
}

// parseCommandLineArgs parses command line arguments and returns app options
func parseCommandLineArgs() AppOptions {
	darkMode := flag.Bool("d", false, "Enable dark mode (invert 1-bit BMP images)")
	showVersion := flag.Bool("v", false, "Show version information")
	verbose := flag.Bool("verbose", true, "Enable verbose output")
	quiet := flag.Bool("q", false, "Quiet mode (disable verbose output)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("trmnl-display version %s (commit: %s, built: %s)\n",
			version, commit, buildDate)
		os.Exit(0)
	}

	return AppOptions{
		DarkMode: *darkMode,
		Verbose:  *verbose && !*quiet,
	}
}

func processNextImage(tmpDir, apiKey string, options AppOptions) {
	// Use defer and recover to handle any panics
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic: %v\n", r)
			time.Sleep(60 * time.Second)
		}
	}()

	// Get the TRMNL display
	req, err := http.NewRequest("GET", "https://usetrmnl.com/api/display", nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		time.Sleep(60 * time.Second)
		return
	}

	req.Header.Add("access-token", apiKey)
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
		fmt.Printf("Error fetching display: status code %d\n", resp.StatusCode)
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

	// Sleep for the refresh rate
	time.Sleep(time.Duration(refreshRate) * time.Second)
}

func displayImage(imagePath string, options AppOptions) error {
	// Open the image file
	file, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("error opening image file: %v", err)
	}
	defer file.Close()

	if options.Verbose {
		fmt.Printf("Reading image from %s\n", imagePath)
	}

	// Get image format
	format, err := getImageFormat(file)
	if err != nil {
		return fmt.Errorf("error determining image format: %v", err)
	}
	if options.Verbose {
		fmt.Printf("Detected image format: %s\n", format)
	}

	// Reset file position after checking format
	file.Seek(0, 0)

	var img image.Image
	// Try standard decoding first
	img, format, err = image.Decode(file)
	// If standard decoding fails for BMP, try our custom decoder
	if err != nil && format == "bmp" {
		if options.Verbose {
			fmt.Printf("Standard BMP decoder failed: %v\n", err)
			fmt.Printf("Trying custom BMP decoder...\n")
		}
		file.Seek(0, 0)
		img, err = decodeCustomBMP(file, options.DarkMode)
		if err != nil {
			return fmt.Errorf("both standard and custom BMP decoders failed: %v", err)
		}
		if options.Verbose {
			fmt.Printf("Successfully decoded image with custom BMP decoder\n")
		}
	} else if err != nil {
		return fmt.Errorf("error decoding image format '%s': %v", format, err)
	} else if options.Verbose {
		fmt.Printf("Successfully decoded image as %s\n", format)
	}

	// Verify we still have the lock before proceeding
	if fbLock != nil && !fbLock.Acquired {
		return fmt.Errorf("lost framebuffer lock, cannot continue")
	}

	// Switch to tty1 so the framebuffer becomes active
	err = exec.Command("chvt", "1").Run()
	if err != nil {
		fmt.Printf("Error switching VT to tty1: %v\n", err)
	}

	// Open the framebuffer
	fb, err := framebuffer.Open("/dev/fb0")
	if err != nil {
		return fmt.Errorf("error opening framebuffer: %v", err)
	}
	defer fb.Close()

	// Get framebuffer bounds
	fbBounds := fb.Bounds()
	if options.Verbose {
		fmt.Printf("Framebuffer bounds: %v\n", fbBounds)
	}

	// Scale the image to fill the entire framebuffer
	targetRect := fbBounds
	scaledImg := image.NewRGBA(targetRect)
	imagedraw.NearestNeighbor.Scale(scaledImg, targetRect, img, img.Bounds(), imagedraw.Over, nil)

	// Draw the scaled image to the framebuffer
	draw.Draw(fb, targetRect, scaledImg, image.Point{}, draw.Src)

	// Flush the framebuffer if necessary
	if fbFlusher, ok := interface{}(fb).(interface{ Flush() error }); ok {
		fbFlusher.Flush()
	}

	if options.Verbose {
		fmt.Println("Image drawing completed (full screen)")
	}
	return nil
}

// decodeCustomBMP attempts to decode a BMP file using a simplified approach
// that can handle some BMP variants that the standard library cannot, including 1-bit BMPs.
func decodeCustomBMP(file *os.File, darkMode bool) (image.Image, error) {
	// Read the entire file
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("error getting file info: %v", err)
	}

	fileSize := fileInfo.Size()
	data := make([]byte, fileSize)
	_, err = file.Read(data)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	// Check BMP signature
	if data[0] != 'B' || data[1] != 'M' {
		return nil, fmt.Errorf("invalid BMP signature")
	}

	// Parse header information
	dataOffset := int(uint32(data[10]) | uint32(data[11])<<8 | uint32(data[12])<<16 | uint32(data[13])<<24)
	headerSize := int(uint32(data[14]) | uint32(data[15])<<8 | uint32(data[16])<<16 | uint32(data[17])<<24)
	width := int(int32(uint32(data[18]) | uint32(data[19])<<8 | uint32(data[20])<<16 | uint32(data[21])<<24))
	if width < 0 {
		width = -width
	}
	height := int(int32(uint32(data[22]) | uint32(data[23])<<8 | uint32(data[24])<<16 | uint32(data[25])<<24))
	isBottomUp := true
	if height < 0 {
		height = -height
		isBottomUp = false
	}
	bitsPerPixel := int(uint16(data[28]) | uint16(data[29])<<8)
	var numColors int
	if headerSize >= 36 && len(data) > 49 {
		numColors = int(uint32(data[46]) | uint32(data[47])<<8 | uint32(data[48])<<16 | uint32(data[49])<<24)
	}
	if numColors == 0 && bitsPerPixel <= 8 {
		numColors = 1 << uint(bitsPerPixel)
	}

	fmt.Printf("BMP Info: width=%d, height=%d, bitsPerPixel=%d, dataOffset=%d, headerSize=%d, numColors=%d\n",
		width, height, bitsPerPixel, dataOffset, headerSize, numColors)

	// Create a new RGBA image
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Calculate row padding (BMP rows are aligned to 4 bytes)
	rowSize := ((width*bitsPerPixel + 31) / 32) * 4

	// For 1-bit (and other indexed) BMPs, read the colour palette
	var palette []color.RGBA
	if bitsPerPixel == 1 || bitsPerPixel == 4 || bitsPerPixel == 8 {
		paletteOffset := 14 + headerSize
		palette = make([]color.RGBA, numColors)
		for i := 0; i < numColors && paletteOffset+i*4+2 < len(data); i++ {
			b := data[paletteOffset+i*4]
			g := data[paletteOffset+i*4+1]
			r := data[paletteOffset+i*4+2]
			palette[i] = color.RGBA{r, g, b, 255}
		}
		if len(palette) < 2 {
			// Default palette for 1-bit BMP: black and white
			palette = []color.RGBA{
				{0, 0, 0, 255},
				{255, 255, 255, 255},
			}
		}

		// Apply dark mode inversion to 1-bit BMPs if enabled
		if darkMode && bitsPerPixel == 1 && len(palette) == 2 {
			fmt.Println("Applying dark mode inversion to 1-bit BMP")
			// Swap the colors in the palette
			palette[0], palette[1] = palette[1], palette[0]
		}

		fmt.Printf("Palette: %v\n", palette)
	}

	// Read pixel data
	for y := 0; y < height; y++ {
		srcY := y
		if isBottomUp {
			srcY = height - 1 - y
		}

		for x := 0; x < width; x++ {
			var col color.RGBA

			switch bitsPerPixel {
			case 24, 32:
				pos := dataOffset + srcY*rowSize + x*bitsPerPixel/8
				if pos+3 > len(data) {
					continue
				}
				b := data[pos]
				g := data[pos+1]
				r := data[pos+2]
				a := uint8(255)
				if bitsPerPixel == 32 && pos+3 < len(data) {
					a = data[pos+3]
				}
				col = color.RGBA{r, g, b, a}
			case 16:
				pos := dataOffset + srcY*rowSize + x*2
				if pos+1 >= len(data) {
					continue
				}
				value := uint16(data[pos]) | uint16(data[pos+1])<<8
				r := uint8((value>>11)&0x1F) << 3
				g := uint8((value>>5)&0x3F) << 2
				b := uint8(value&0x1F) << 3
				col = color.RGBA{r, g, b, 255}
			case 8:
				pos := dataOffset + srcY*rowSize + x
				if pos >= len(data) {
					continue
				}
				index := data[pos]
				if int(index) < len(palette) {
					col = palette[index]
				} else {
					col = color.RGBA{0, 0, 0, 255}
				}
			case 4:
				pos := dataOffset + srcY*rowSize + x/2
				if pos >= len(data) {
					continue
				}
				var index uint8
				if x%2 == 0 {
					index = (data[pos] >> 4) & 0x0F
				} else {
					index = data[pos] & 0x0F
				}
				if int(index) < len(palette) {
					col = palette[index]
				} else {
					col = color.RGBA{0, 0, 0, 255}
				}
			case 1:
				bytePos := dataOffset + srcY*rowSize + x/8
				bitPos := 7 - (x % 8)
				if bytePos >= len(data) {
					continue
				}
				bit := (data[bytePos] >> bitPos) & 1
				if int(bit) < len(palette) {
					col = color.RGBA{palette[bit].R, palette[bit].G, palette[bit].B, 255}
				} else {
					if bit == 0 {
						col = color.RGBA{0, 0, 0, 255}
					} else {
						col = color.RGBA{255, 255, 255, 255}
					}
				}
			default:
				return nil, fmt.Errorf("unsupported BMP bit depth: %d", bitsPerPixel)
			}

			// Use the standard Set method.
			img.Set(x, y, col)
		}
	}

	return img, nil
}

// getImageFormat determines the image format based on its header.
func getImageFormat(file *os.File) (string, error) {
	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return "", err
	}

	signatures := map[string][]byte{
		"jpeg": {0xFF, 0xD8},
		"png":  {0x89, 0x50, 0x4E, 0x47},
		"gif":  {0x47, 0x49, 0x46},
		"bmp":  {0x42, 0x4D},
	}

	for format, signature := range signatures {
		match := true
		for i, b := range signature {
			if buffer[i] != b {
				match = false
				break
			}
		}
		if match {
			return format, nil
		}
	}

	return "unknown", nil
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

// checkDisplayServer is a placeholder for checking if a display server is running.
func checkDisplayServer() {
	// Add code here to check for X server, Wayland, etc., if needed.
	fmt.Println("Display server check not implemented, assuming framebuffer usage")
}

// listFramebufferDevices lists available framebuffer devices.
func listFramebufferDevices() {
	files, err := filepath.Glob("/dev/fb*")
	if err != nil {
		fmt.Printf("Error listing framebuffer devices: %v\n", err)
		return
	}
	fmt.Printf("Found framebuffer devices: %v\n", files)
}
