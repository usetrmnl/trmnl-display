# TRMNL Display

TRMNL Display is a lightweight, terminal-based application designed to display dynamic images directly on framebuffer-enabled devices, such as Raspberry Pi. It fetches images from the TRMNL API and renders them directly to the framebuffer, providing a seamless display experience without requiring a traditional desktop environment.

## Features

- Direct framebuffer image rendering.
- Supports JPEG, PNG, and BMP image formats.
- Custom handling for BMP images, including 1-bit BMPs with dark mode inversion.
- Configurable refresh rates.
- Easy configuration through environment variables or interactive prompts.
- Automated cross-compilation script for various Raspberry Pi models and architectures.

## Requirements

- Go 1.19 or higher
- Framebuffer-enabled Linux device (e.g., Raspberry Pi)
- Internet connection for fetching images

## Installation

Clone the repository:

```bash
git clone https://github.com/usetrmnl/trmnl-display.git
cd trmnl-display
```

Build the binary locally (for your current platform):

```bash
go build -o trmnl-display ./trmnl-display.go
```

## Cross-compilation (Raspberry Pi)

To build for Raspberry Pi architectures, run the provided `build.sh` script:

```bash
chmod +x build.sh
./build.sh
```

This script will automatically handle cross-compilation and upload binaries to your specified AWS S3 bucket.

## Usage

Ensure your TRMNL API key is set either in the environment variable or via the interactive prompt:

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

TRMNL Display stores configuration files in:

```
~/.trmnl/config.json
```

This file will store your API key for convenience.

## Licence

TRMNL Display is licensed under the MIT Licence. See [LICENSE](./LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or pull request on GitHub.
