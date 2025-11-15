#!/bin/bash
set -e
# This script builds the trmnl-epaper binary for multiple Raspberry Pi architectures using cross-compilation.

# save the current directory
  pushd .
# Install the required components
  sudo apt install git gpiod libgpiod-dev golang-go -y

# clone and build the epaper and PNG support
  mkdir -p $HOME/Projects
  cd $HOME/Projects
  if [ -d $HOME/Projects/bb_epaper ]; then
      echo "bb_epaper already exists"
  else
      git clone https://github.com/bitbank2/bb_epaper
  fi

  if [ -d $HOME/Projects/PNGdec ]; then
      echo "PNGdec already exists"
  else
      git clone https://github.com/bitbank2/PNGdec
  fi

  if [ -d $HOME/Projects/JPEGDEC ]; then
      echo "JPEGDEC already exists"
  else
      git clone https://github.com/bitbank2/JPEGDEC
  fi

  cd PNGdec/linux
  make
  cd ../../JPEGDEC/linux
  make
  cd ../../bb_epaper/rpi
  make
  # cd examples/show_img
  make
# restore the original directory
  popd
  echo "Select your display device:"
  echo "  1) framebuffer (HDMI/LCD)"
  echo "  2) Waveshare e-paper HAT"
  read n
  JSTART=$(printf "{\n        \"adapter\": \"")
  case $n in
	  1) echo 0 | sudo tee /sys/class/graphics/fbcon/cursor_blink
	     JADAPTER="framebuffer";;
	  2) JADAPTER="waveshare_2";;
	  *) echo "Invalid option" ; exit 1;;
  esac
  JEND=$(printf "\",\n        \"stretch\": \"aspectfill\",\n        \"panel_1bit\": \"EP75_800x480_GEN2\",\n        \"panel_2bit\": \"EP75_800x480_4GRAY_GEN2\"\n}\n")
  printf '%s%s%s' "$JSTART" "$JADAPTER" "$JEND" > epaper.json

  echo "Compiling TRMNL go program..."
  go build -o trmnl-display ./trmnl-display.go
  
  echo "Build complete. Run trmnl-display to start."

