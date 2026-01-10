#!/bin/bash
set -e
# This script builds the trmnl-epaper binary for multiple Raspberry Pi architectures using cross-compilation.

# save the current directory
  pushd .
# Install the required components
  sudo apt install git gpiod libgpiod-dev golang-go -y

# clone and build the epaper and image file support
  mkdir -p $HOME/Projects
  mkdir -p $HOME/.config/trmnl
  cd $HOME/Projects
  if [ -d $HOME/Projects/bb_epaper ]; then
      echo "bb_epaper already cloned, updating to latest..."
      cd bb_epaper
      git pull
      cd ..
  else
      git clone https://github.com/bitbank2/bb_epaper
  fi

  if [ -d $HOME/Projects/PNGdec ]; then
      echo "PNGdec already cloned, updating to latest..."
      cd PNGdec
      git pull
      cd ..
  else
      git clone https://github.com/bitbank2/PNGdec
  fi

  if [ -d $HOME/Projects/JPEGDEC ]; then
      echo "JPEGDEC already cloned, updating to latest..."
      cd JPEGDEC
      git pull
      cd ..
  else
      git clone https://github.com/bitbank2/JPEGDEC
  fi

  cd PNGdec/linux
  make
  cd ../../JPEGDEC/linux
  make
  cd ../../bb_epaper/rpi
  make
  cd examples/show_img
  make
# restore the original directory
  popd
  echo "Select your display device:"
  echo "  1) framebuffer (HDMI/LCD)"
  echo "  2) Waveshare e-paper HAT"
  echo "  3) Pimoroni Inky Impression Spectra 7.3"
  read n
  JSTART=$(printf "{\n        \"adapter\": \"")
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
	  *) echo "Invalid option" ; exit 1;;
  esac
  JEND=$(printf "\",\n        \"stretch\": \"aspectfill\",\n        \"panel_1bit\": \"$PANEL\",\n        \"panel_2bit\": \"$PANEL2\"\n}\n")
  printf '%s%s%s' "$JSTART" "$JADAPTER" "$JEND" > $HOME/.config/trmnl/show_img.json

  echo "Compiling TRMNL go program..."
  go build -o trmnl-display ./trmnl-display.go
  
  echo "Build complete. Run trmnl-display to start."

