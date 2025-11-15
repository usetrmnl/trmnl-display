# TRMNL Display

TRMNL Display is a lightweight, Linux command line application designed to display dynamic images directly on both framebuffer (LCD/HDMI) and SPI e-paper displays like the one in the TRMNL og. It fetches images from the TRMNL API (or your own self-hosted sever) and renders them directly to either a framebuffer or e-paper, providing a seamless display experience without requiring a traditional desktop environment.

## Features

- Direct framebuffer or e-paper image rendering.
- Supports JPEG, PNG, and BMP image formats.
- 1-bit images support optional dark mode inversion.
- Configurable refresh rates.
- Easy configuration through environment variables or interactive prompts.

## Requirements

- Linux SBC (Raspberry Pi, Orange Pi, etc)
- Go 1.24 or higher (minimum version required)
- framebuffer-enabled display
- or e-paper display with SPI connection
- Internet connection for fetching images

## Installation

Clone the repository:

```bash
git clone https://github.com/usetrmnl/trmnl-display.git
cd trmnl-display
```

Run the provided `build.sh` script:

```bash
./build.sh
```

You'll see a message:
```bash
Select your display device:
  1) framebuffer (HDMI/LCD)
  2) Waveshare e-paper HAT
```

Input "1" or "2", then press enter. The script will complete with the following:

```bash
Compiling TRMNL go program...
Build complete. Run trmnl-display to start.
```

## Usage
Navigate to wherever you cloned the `trmnl-display` repository.

Run the application:

```bash
./trmnl-display
```

On the first run you'll be asked to provide your API Key. If you're using TRMNL's native application at usetrmnl.com, find it at https://usetrmnl.com/devices/current/edit.

If you're using a [BYOS solution](https://docs.usetrmnl.com/go/diy/byos), find your API key from that implementation's settings screen. You will also need to change the `base_url` to point to your server. See **Configuration** for more details.

Optional flags:

- Enable dark mode (inverts all pixels):

```bash
./trmnl-display -d
```

## Background Usage
Navigate to wherever you cloned the `trmnl-display` repository.

Run the application:
```bash
nohup ./trmnl-display &
```

This lets you escape the command (`ctrl+c`) and close your session without terminating the script.

## Configuration

TRMNL Display by default stores configuration files in:

```
~/.config/trmnl/config.json
```

This file stores your API Key and other preferences. You may also need to provide your MAC address with key `device_id` for BYOS clients that require it to be paired with an API Key in the request headers.

If you selected an e-paper display device during Installation, there will be an additional json configuration (epaper.json) which configures the e-paper GPIO connection and panel type. See the example file provided with this repo.

## License

TRMNL Display is licensed under the MIT License. See [LICENSE](./LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or pull request on GitHub.
