# TRMNL Display

TRMNL Display is a lightweight, terminal-based application designed to display dynamic images directly on framebuffer-enabled devices, such as Raspberry Pi with HDMI or e-paper displays. It fetches images from the TRMNL API (or your own self-hosted sever) and renders them directly to the framebuffer, providing a seamless display experience without requiring a traditional desktop environment.

## Features

- Direct framebuffer image rendering.
- Supports JPEG, PNG, and BMP image formats.
- Custom handling for BMP images, including 1-bit BMPs with dark mode inversion.
- Configurable refresh rates.
- Easy configuration through environment variables or interactive prompts.

## Requirements

- Go 1.24 or higher (minimum version required)
- Framebuffer-enabled Linux device (Raspberry Pi, Orange Pi, etc)
- HDMI display or e-paper display
- Internet connection for fetching images

## Installation

Clone the repository:

```bash
git clone https://github.com/usetrmnl/trmnl-display.git
cd trmnl-display
```

#### E-paper Display

Run the provided `build_epd.sh` script:

```bash
./build_epd.sh

# logs
# sudo cp show_png /usr/local/bin
# Enabling SPI bus
# ~/Desktop/trmnl-display
# Compiling TRMNL go program...
```

When it completes you'll a message: `Build complete. Run trmnl-epaper to start`.

#### HDMI Display

To build for Raspberry Pi architectures, run the provided `build.sh` script:

```bash
chmod +x build.sh
./build.sh
```

Otherwise, build the binary locally (for your current platform):

```bash
go build -o trmnl-display
```

## Usage
Navigate to wherever you cloned the `trmnl-display` repository.

#### E-paper Display
Run the application:

```bash
./trmnl-epaper
```

On the first run you'll be asked to provide your API Key. If you're using TRMNL's native application at usetrmnl.com, find it at https://usetrmnl.com/devices/current/edit.

If you're using a [BYOS solution](https://docs.usetrmnl.com/go/diy/byos), find your API key from that implementation's settings screen. You will also need to change the `base_url` to point to your server. See **Configuration** for more details.

#### HDMI Display
Ensure your TRMNL API key is set either in the **Configuration** or as an environment variable:

```bash
export TRMNL_API_KEY="your_api_key_here"
```

Run the application:

```bash
./trmnl-display
```

Optional flags:

- Enable dark mode (invert 1-bit BMP images):

```bash
./trmnl-display -d
```

## Configuration

TRMNL Display by default stores configuration files in:

```
~/.config/trmnl/config.json
```

This file stores your API Key and other preferences. You may also need to provide your MAC address with key `device_id` for BYOS clients that require it to be paired with an API Key in the request headers.

## License

TRMNL Display is licensed under the MIT License. See [LICENSE](./LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or pull request on GitHub.
