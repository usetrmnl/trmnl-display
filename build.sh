#!/bin/bash
set -e
# This script builds the trmnl-display binary and sets up the chosen display backend.
#
# Two rendering backends are supported:
#   * bb_epaper (C)  - framebuffer (HDMI/LCD), Waveshare HATs, Pimoroni Inky 7.3" Spectra
#   * inky (Python)  - Pimoroni Inky Impression 13.3" and other Inky boards (via Pimoroni's
#                      `inky` library, which already supports them)

# Remember where the repo lives so we can reference inky_display.py with an absolute path.
REPO_DIR="$(pwd)"

# Install the base requirements (Go toolchain + git)
  sudo apt install git gpiod libgpiod-dev golang-go -y

  mkdir -p $HOME/Projects
  mkdir -p $HOME/.config/trmnl

  echo "Select your display device:"
  echo "  1) framebuffer (HDMI/LCD)"
  echo "  2) Waveshare e-paper HAT"
  echo "  3) Pimoroni Inky Impression Spectra 7.3 (bb_epaper backend)"
  echo "  4) Pimoroni Inky Impression 13.3 / other Inky board (inky Python backend)"
  read n

  case $n in
    1|2|3)
      # ---- bb_epaper (C) backend ---------------------------------------------
      # clone and build the epaper and image file support
      pushd .
      cd $HOME/Projects
      for repo in bb_epaper PNGdec JPEGDEC; do
          if [ -d "$HOME/Projects/$repo" ]; then
              echo "$repo already cloned, updating to latest..."
              (cd "$repo" && git pull)
          else
              git clone "https://github.com/bitbank2/$repo"
          fi
      done

      cd PNGdec/linux
      make
      cd ../../JPEGDEC/linux
      make
      cd ../../bb_epaper/rpi
      make
      cd examples/show_img
      make
      popd

      PANEL2="EP75_800x480_4GRAY_GEN2"
      case $n in
        1) echo 0 | sudo tee /sys/class/graphics/fbcon/cursor_blink
           PANEL="EP75_800x480_GEN2"
           JADAPTER="framebuffer";;
        2) JADAPTER="waveshare_2"
           PANEL="EP75_800x480_GEN2";;
        3) JADAPTER="pimoroni"
           PANEL2="EP73_SPECTRA_800x480"
           PANEL="EP73_SPECTRA_800x480";;
      esac
      JSTART=$(printf "{\n        \"adapter\": \"")
      JEND=$(printf "\",\n        \"stretch\": \"aspectfill\",\n        \"panel_1bit\": \"$PANEL\",\n        \"panel_2bit\": \"$PANEL2\"\n}\n")
      printf '%s%s%s' "$JSTART" "$JADAPTER" "$JEND" > $HOME/.config/trmnl/show_img.json
      ;;

    4)
      # ---- inky (Python) backend ---------------------------------------------
      echo "Setting up the Pimoroni inky Python backend..."
      sudo apt install python3 python3-venv python3-dev python3-pip -y

      # Enable the buses the Inky boards need: SPI for pixel data and I2C so the
      # inky library can auto-detect the board from its EEPROM.
      if command -v raspi-config >/dev/null 2>&1; then
          sudo raspi-config nonint do_spi 0 || true
          sudo raspi-config nonint do_i2c 0 || true
      fi

      # The Impression 13.3" drives its two chip-selects from GPIO, so free the
      # hardware SPI CS pins. Harmless for the smaller single-CS Inky boards.
      CONFIG_TXT=/boot/firmware/config.txt
      [ -f "$CONFIG_TXT" ] || CONFIG_TXT=/boot/config.txt
      if [ -f "$CONFIG_TXT" ] && ! grep -q "^dtoverlay=spi0-0cs" "$CONFIG_TXT"; then
          echo "dtoverlay=spi0-0cs" | sudo tee -a "$CONFIG_TXT"
          echo "Added 'dtoverlay=spi0-0cs' to $CONFIG_TXT - reboot required for it to take effect."
      fi

      # Install the inky library into an isolated virtualenv (Pi OS Bookworm's
      # system Python is externally-managed). --system-site-packages lets it
      # reuse any system GPIO/SPI packages that are present.
      VENV="$HOME/Projects/trmnl-inky-venv"
      python3 -m venv --system-site-packages "$VENV"
      "$VENV/bin/pip" install --upgrade pip
      "$VENV/bin/pip" install "inky[rpi]" pillow numpy

      # Point trmnl-display at the Python renderer, preserving any credentials
      # already present in config.json.
      DISPLAY_CMD="$VENV/bin/python3 $REPO_DIR/inky_display.py"
      "$VENV/bin/python3" - "$HOME/.config/trmnl/config.json" "$DISPLAY_CMD" <<'PY'
import json, os, sys
path, cmd = sys.argv[1], sys.argv[2]
cfg = {}
if os.path.exists(path):
    try:
        with open(path) as f:
            cfg = json.load(f)
    except Exception:
        cfg = {}
cfg["display_command"] = cmd
with open(path, "w") as f:
    json.dump(cfg, f, indent=2)
PY
      echo "inky backend configured."
      echo "  render command: $DISPLAY_CMD"
      echo "  smoke test (shows a colour test pattern on the panel):"
      echo "    $VENV/bin/python3 $REPO_DIR/inky_display.py selftest"
      ;;

    *) echo "Invalid option" ; exit 1;;
  esac

  echo "Compiling TRMNL go program..."
  go build -o trmnl-display ./trmnl-display.go

  echo "Build complete. Run trmnl-display to start."
