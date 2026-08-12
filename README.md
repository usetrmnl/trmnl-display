# TRMNL Display

TRMNL Display is a lightweight, Linux command line application designed to display dynamic images directly on LCD/HDMI and SPI e-paper displays like the one in the TRMNL OG. It fetches images from the TRMNL API (or [your own self-hosted server](https://docs.trmnl.com/go/diy/byos)) and renders them directly to either a video display or e-paper, providing a seamless display experience without requiring a traditional desktop environment.

## Features

- LCD/HDMI or e-paper image rendering.
- Supports JPEG, PNG, and BMP image formats.
- 1-bit images support optional dark mode inversion.
- Configurable refresh rates.
- Easy configuration through environment variables or interactive prompts.

## Requirements

- Linux SBC (Raspberry Pi, Orange Pi, etc)
- LCD/HDMI (Terminal or GUI desktop)
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
After installing the required dependent libraries and building the
trmnl_display application, you'll see a menu and prompt: 
```bash
Select your display device:
  1) framebuffer (HDMI/LCD)
  2) Waveshare SPI e-paper HAT
  3) Pimoroni Inky Impression Spectra 7.3
  4) Pimoroni Inky Impression Spectra 13.3
  5) Waveshare IT8951 7.8 or 10.3 1872x1440
```
  
Input a value (number 1-5), then press enter. Next you'll be asked to enter your Device API key. If you're using TRMNL's native application, visit [https://trmnl.com/devices/current/edit](https://trmnl.com/devices/current/developer/edit) to find it.

If you're using a [BYOS solution](https://docs.trmnl.com/go/diy/byos), find your API key from that implementation's settings screen. You will also need to change the `base_url` to point to your server. See **Configuration** for more details.

The script will complete with the following:

```bash
Build complete. Run trmnl_display to start.
```

## Usage

The trmnl_display program is copied to /usr/local/bin, so you can run it from anywhere. Simply type:

```bash
trmnl_display
```


To skip to the next item in your playlist, press the ENTER key. To exit the program, press the ESC key.

```bash
Keypress...skipping to next update
Displayed: /tmp/trmnl-display3898330261/plugin-b67875-1763221411
EPD update completed
```

Optional flags:

- Enable dark mode (inverts all pixels):

```bash
trmnl_display -d
```

## Background Usage
Navigate to wherever you cloned the `trmnl-display` repository.

Run the application:
```bash
nohup ./trmnl-display &
```

This lets you escape the command (`ctrl+c`) and close your session without terminating the script.

**Background + Automatic Reboot**

To restart `trmnl_display` whenever your device is turned on, access your crontab editor with `crontab -e`. You may be required to set an editor (1, 2, 3), then press enter.

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
@reboot sleep 15 && nohup trmnl_display > /home/$(whoami)/.config/trmnl/logfile.log 2>&1 &
```

The `sleep 15` intends to ensure your network configuration is ready before `trmnl-display` makes an HTTP request to your playlist.

Confirm this works by running `sudo reboot`, which should momentarily trigger an automatic screen refresh.

## Configuration

TRMNL Display by default stores the following two configuration files in:

```
~/.config/trmnl/config.json
~/.config/trmnl/show_img.json
```

config.json stores your Device API Key and other preferences. You may also need to provide your MAC address with key `device_id` for BYOS clients that require it to be paired with an API Key in the request headers.

show_img.json stores the configuration for your e-paper or framebuffer display device. It will be created during Installation (one of the final steps in build.sh).

## License

TRMNL Display is licensed under the MIT License. See [LICENSE](./LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or pull request on GitHub.
