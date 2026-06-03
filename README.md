# TRMNL Display

TRMNL Display is a lightweight, Linux command line application designed to display dynamic images directly on both framebuffer (LCD/HDMI) and SPI e-paper displays like the one in the TRMNL OG. It fetches images from the TRMNL API (or [your own self-hosted server](https://docs.usetrmnl.com/go/diy/byos)) and renders them directly to either a framebuffer or e-paper, providing a seamless display experience without requiring a traditional desktop environment.

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
  3) Pimoroni Inky Impression Spectra 7.3 (bb_epaper backend)
  4) Pimoroni Inky Impression 13.3 / other Inky board (inky Python backend)
```

Input "1", "2", "3" or "4", then press enter. The script will complete with the following:

```bash
Compiling TRMNL go program...
Build complete. Run trmnl-display to start.
```

### Display backends

Options 1–3 use the [bb_epaper](https://github.com/bitbank2/bb_epaper) C library (built from source by `build.sh`) via its `show_img` tool.

Option 4 uses Pimoroni's [`inky`](https://github.com/pimoroni/inky) Python library instead, which natively supports the **Inky Impression 13.3" (Spectra 6 / EL133UF1, 1600×1200)** as well as the other Inky boards (5.7", 7.3", pHAT, wHAT, Inky Frame). For this option `build.sh` installs `inky` into a virtualenv (`~/Projects/trmnl-inky-venv`), enables SPI/I2C, adds `dtoverlay=spi0-0cs`, and records the renderer in `config.json` as `display_command`. A **reboot is required** after install for the SPI overlay to take effect.

The Go program treats the renderer as an external command, so the two backends are interchangeable; `config.json`'s optional `display_command` selects which one is used (defaults to `show_img` when unset). Rendering for option 4 is handled by [`inky_display.py`](./inky_display.py).

#### Smoke-testing the Inky backend

Before wiring up the server, you can confirm the panel works with a built-in test pattern (six Spectra-6 colour bars, a centre seam line and corner-to-corner diagonals). On the device:

```bash
~/Projects/trmnl-inky-venv/bin/python3 ./inky_display.py selftest
```

The diagonals should meet cleanly at the centre and the centre line should be unbroken — a kink or break there indicates the two controllers of the 13.3" panel are misaligned. To preview the pattern as a PNG on any machine (no board required), pass a size and output path:

```bash
python3 ./inky_display.py selftest size=1600x1200 out=preview.png
```

## Usage
Navigate to wherever you cloned the `trmnl-display` repository.

Run the application:

```bash
./trmnl-display
```

On the first run you'll be asked to provide your Device API Key. If you're using TRMNL's native application at usetrmnl.com, go to https://usetrmnl.com/devices/current/edit and find the key under the Developer Perks section.

If you're using a [BYOS solution](https://docs.usetrmnl.com/go/diy/byos), find your API key from that implementation's settings screen. You will also need to change the `base_url` to point to your server. See **Configuration** for more details.

To skip to the next item in your playlist, press the `enter` key.

```bash
Keypress...skipping to next update
Displayed: /tmp/trmnl-display3898330261/plugin-b67875-1763221411
EPD update completed
```

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

**Background + Automatic Reboot**

To restart `trmnl-display` whenever your device is turned on, access your crontab editor with `crontab -e`. You may be required to set an editor (1, 2, 3), then press enter.

```bash
crontab -e
no crontab for trmnl - using an empty one
Select an editor.  To change later, run select-editor again.
  1. /bin/nano        <---- easiest
  2. /usr/bin/vim.tiny
  3. /bin/ed

Choose 1-3 [1]:
```

Inside your crontab, paste the following command. Change the path (if applicable) to point to your `trmnl-display` Installation location:

```bash
@reboot sleep 15 && nohup /home/$(whoami)/Desktop/trmnl-display/./trmnl-display > /home/$(whoami)/.config/trmnl/logfile.log 2>&1 &
```

The `sleep 15` intends to ensure your network configuration is ready before `trmnl-display` makes an HTTP request to your playlist.

Confirm this works by running `sudo reboot`, which should momentarily trigger an automatic screen refresh.

## Configuration

TRMNL Display by default stores the following two configuration files in:

```
~/.config/trmnl/config.json
~/.config/trmnl/show_img.json
```

config.json stores your Device API Key and other preferences. You may also need to provide your MAC address with key `device_id` for BYOS clients that require it to be paired with an API Key in the request headers. It may also contain an optional `display_command` key, which selects the rendering backend (see [Display backends](#display-backends)); when unset it defaults to the bb_epaper `show_img` tool.

show_img.json stores the configuration for your e-paper or framebuffer display device when using the bb_epaper backend (options 1–3). It will be created during Installation (one of the final steps in build.sh). The inky Python backend (option 4) does not use this file.

## License

TRMNL Display is licensed under the MIT License. See [LICENSE](./LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or pull request on GitHub.
