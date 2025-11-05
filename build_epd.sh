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
  cd examples/show_png
  make
# restore the original directory
  popd
  echo "Compiling TRMNL go program..."
  go build -o trmnl-epaper ./trmnl-epaper.go
  
  echo "Build complete. Run trmnl-epaper to start."

