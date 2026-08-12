#!/bin/bash
set -e
# This script builds the trmnl-epaper binary for multiple Raspberry Pi architectures using cross-compilation.

# save the current directory
  pushd .
# Install the required components
  sudo apt install git gpiod libgpiod-dev libi2c-dev libcurl4-openssl-dev libsdl2-dev -y

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

  if [ -d $HOME/Projects/bb_scd41 ]; then
      echo "bb_scd41 already cloned, updating to latest..."
      cd bb_scd41
      git pull
      cd ..
  else
      git clone https://github.com/bitbank2/bb_scd41
  fi

  if [ -d $HOME/Projects/bb_temperature ]; then
      echo "bb_temperature already cloned, updating to latest..."
      cd bb_temperature
      git pull
      cd ..
  else
      git clone https://github.com/bitbank2/bb_temperature
  fi
  if [ -d $HOME/Projects/FastEPD ]; then
      echo "FastEPD already cloned, updating to latest..."
      cd FastEPD
      git pull
      cd ..
  else
      git clone https://github.com/bitbank2/FastEPD
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

  if [ -d $HOME/Projects/trmnl_lib ]; then
      echo "trmnl_lib already cloned, updating to latest..."
      cd trmnl_lib
      git pull
      cd ..
  else
      git clone https://github.com/usetrmnl/trmnl_lib
  fi

  cd PNGdec/linux
  make
  cd ../../FastEPD/Linux
  make
  cd ../../JPEGDEC/linux
  make
  cd ../../bb_epaper/rpi
  make
  cd ../../bb_scd41/Linux
  make
  cd ../../bb_temperature/Linux
  make
  cd ../../trmnl_lib/linux
  make
  popd
  echo "Compiling trmnl_display program..."
  make

  echo "Select your display device:"
  echo "  1) framebuffer (HDMI/LCD)"
  echo "  2) Waveshare SPI e-paper HAT"
  echo "  3) Pimoroni Inky Impression Spectra 7.3"
  echo "  4) Pimoroni Inky Impression Spectra 13.3"
  echo "  5) Waveshare IT8951 7.8 or 10.3 1872x1440"
  read n
  JSTART=$(printf "{\n        \"adapter\": \"")
  PANEL2="EP75_800x480_4GRAY"
  case $n in
          1) PANEL="EP75_800x480"
             JADAPTER="framebuffer";;
          2) JADAPTER="waveshare_2"
             echo "  Set switches to: Interface Config (0), Display Config (A)"
             PANEL="EP75_800x480";;
          3) JADAPTER="pimoroni"
             PANEL2="EP73_SPECTRA_800x480"
             PANEL="EP73_SPECTRA_800x480";;
          4) JADAPTER="pimoroni_2"
             PANEL2="EP133_SPECTRA_1200x1600"
             PANEL="EP133_SPECTRA_1200x1600";;
          5) JADAPTER="waveshare_it8951"
             PANEL2="IT8951_1872x1440"
             PANEL="IT8951_1872x1440";;
          *) echo "Invalid option" ; exit 1;;
  esac
  JEND=$(printf "\",\n        \"stretch\": \"aspectfill\",\n        \"panel_1bit\": \"$PANEL\",\n        \"panel_2bit\": \"$PANEL2\"\n}\n")
  printf '%s%s%s' "$JSTART" "$JADAPTER" "$JEND" > $HOME/.config/trmnl/show_img.json
  echo "  Enter your API (device) key"
  read key
  JKEY=$(printf "{\n        \"api_key\": \"")
  JURL=$(printf "\",\n        \"base_url\": \"https://trmnl.app\"\n}\n")
  printf '%s%s%s' "$JKEY" "$key" "$JURL" > $HOME/.config/trmnl/config.json
  echo "Build complete. Run trmnl_display to start."

