#!/bin/bash
set -e
# This script builds the trmnl-epaper binary for multiple Raspberry Pi architectures using cross-compilation.
# AWS CLI is required for uploading to S3.

# Install the required components
  sudo apt install git gpiod libgpiod-dev golang-go awscli -y

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

  cd PNGdec/linux
  make
  cd ../../bb_epaper/rpi
  make
  cd examples/show_png
  make

# Check for required commands.
command -v go >/dev/null 2>&1 || { echo >&2 "go is required but not installed. Aborting."; exit 1; }
command -v aws >/dev/null 2>&1 || { echo >&2 "aws CLI is required but not installed. Aborting."; exit 1; }

# S3 bucket and URL information
S3_BUCKET="byod.usetrmnl.com"
S3_URL="http://byod.usetrmnl.com.s3-website-us-east-1.amazonaws.com"

# Create a temporary directory for the builds
BUILD_DIR=$(mktemp -d)
echo "Build directory: $BUILD_DIR"

# Define the target architectures for Raspberry Pi models with cross-compiler settings
declare -A targets=(
  ["rpi1-zero"]="arm:6:arm-linux-gnueabihf-gcc:armv6"        # Raspberry Pi 1 (A, B, A+, B+), Zero, Zero W
  ["rpi2v1.1"]="arm:7:arm-linux-gnueabihf-gcc:armv7"         # Raspberry Pi 2 (v1.1)
  ["rpi2v1.2-3"]="arm:7:arm-linux-gnueabihf-gcc:armv7"       # Raspberry Pi 2 (v1.2), 3, 3+, CM3
  ["rpi4-32bit"]="arm:7:arm-linux-gnueabihf-gcc:armv7"       # Raspberry Pi 4, 400, CM4 (32-bit OS)
  ["rpi4-64bit"]="arm64::aarch64-linux-gnu-gcc:aarch64"      # Raspberry Pi 4, 400, CM4 (64-bit OS)
  ["rpi5-64bit"]="arm64::aarch64-linux-gnu-gcc:aarch64"      # Raspberry Pi 5 (64-bit OS)
  ["rpi-64bit-kernel-32bit-userspace"]="arm:7:arm-linux-gnueabihf-gcc:armv7-64k"  # 64-bit kernel, 32-bit userspace
)

# First check if cross-compilers are installed
check_cross_compiler() {
  local cc=$1
  if [ -z "$cc" ]; then return 0; fi

  if ! command -v $cc >/dev/null 2>&1; then
    echo "Warning: Cross-compiler $cc not found. Installing cross-compilers may be necessary."
    echo "For Debian/Ubuntu: sudo apt-get install gcc-arm-linux-gnueabihf gcc-aarch64-linux-gnu"
    echo "For other distros, please install the appropriate cross-compiler packages."
    echo "Attempting to build without cross-compiler..."
    return 1
  fi
  return 0
}

# Build for all target architectures
for target in "${!targets[@]}"; do
  # Parse the architecture, ARM version, cross-compiler, and binary suffix
  IFS=':' read -r ARCH ARM_VERSION CC BIN_SUFFIX <<< "${targets[$target]}"

  echo "Building for $target (GOARCH=$ARCH GOARM=$ARM_VERSION)"

  # Check if cross-compiler is available
  if ! check_cross_compiler $CC; then
    echo "Skipping $target build due to missing cross-compiler"
    continue
  fi

  # Set binary name
  BIN_NAME="trmnl-epaper-linux-$BIN_SUFFIX"

  # Set environment variables for cross-compilation
  export GOOS=linux
  export GOARCH=$ARCH
  if [[ -n "$ARM_VERSION" ]]; then
    export GOARM=$ARM_VERSION
  else
    unset GOARM
  fi

  # Enable CGO and set CC for cross-compilation
  export CGO_ENABLED=1
  export CC=$CC

  echo "Building $BIN_NAME with GOARCH=$GOARCH GOARM=$GOARM CC=$CC (statically linked)"

  # Attempt static linking explicitly
  if go build -a -ldflags '-extldflags "-static"' -o "$BUILD_DIR/$BIN_NAME" ./trmnl-epaper.go; then
    echo "Static build successful for $BIN_NAME"
  else
    echo "Static build failed, attempting fallback without static flags..."
    if go build -o "$BUILD_DIR/$BIN_NAME" ./trmnl-epaper.go; then
      echo "Fallback build successful for $BIN_NAME (dynamic linking)"
    else
      echo "Failed to build for $target"
      continue
    fi
  fi

  # Ensure binary is executable
  chmod +x "$BUILD_DIR/$BIN_NAME"

  # Verify binary linkage
  echo "Linkage type for $BIN_NAME:"
  file "$BUILD_DIR/$BIN_NAME"

  # Upload to S3
  echo "Uploading $BIN_NAME to S3 bucket: $S3_BUCKET"
  aws s3 cp "$BUILD_DIR/$BIN_NAME" "s3://$S3_BUCKET/$BIN_NAME"

  echo "Upload complete! Binary available at: $S3_URL/$BIN_NAME"
done

# Also build for x86_64 for completeness
echo "Building for x86_64"
export GOOS=linux
export GOARCH=amd64
unset GOARM
BIN_NAME="trmnl-display-linux-amd64"

# Use native compilation for x86_64 if we're on an x86_64 system
if [[ "$(uname -m)" == "x86_64" ]]; then
  # For native build, we can just use the system compiler
  export CGO_ENABLED=1
  unset CC
  echo "Using native compilation for x86_64"
  if go build -o "$BUILD_DIR/$BIN_NAME" ./trmnl-display.go; then
    chmod +x "$BUILD_DIR/$BIN_NAME"
    echo "Uploading $BIN_NAME to S3 bucket: $S3_BUCKET"
    aws s3 cp "$BUILD_DIR/$BIN_NAME" "s3://$S3_BUCKET/$BIN_NAME"
    echo "Upload complete! Binary is now available at: $S3_URL/$BIN_NAME"
  else
    echo "Failed to build for x86_64. Trying with CGO disabled..."
    export CGO_ENABLED=0
    if go build -o "$BUILD_DIR/$BIN_NAME" ./trmnl-display.go; then
      chmod +x "$BUILD_DIR/$BIN_NAME"
      echo "Uploading $BIN_NAME to S3 bucket: $S3_BUCKET"
      aws s3 cp "$BUILD_DIR/$BIN_NAME" "s3://$S3_BUCKET/$BIN_NAME"
      echo "Upload complete! Binary is now available at: $S3_URL/$BIN_NAME"
    else
      echo "Failed to build for x86_64"
    fi
  fi
else
  # We're on a non-x86_64 system, so we need a cross-compiler
  echo "Non-x86_64 system detected, attempting cross-compilation for x86_64"
  echo "This may fail without the appropriate cross-compiler."
  export CGO_ENABLED=0  # Disable CGO for cross-compilation
  if go build -o "$BUILD_DIR/$BIN_NAME" ./trmnl-display.go; then
    chmod +x "$BUILD_DIR/$BIN_NAME"
    echo "Uploading $BIN_NAME to S3 bucket: $S3_BUCKET"
    aws s3 cp "$BUILD_DIR/$BIN_NAME" "s3://$S3_BUCKET/$BIN_NAME"
    echo "Upload complete! Binary is now available at: $S3_URL/$BIN_NAME"
  else
    echo "Failed to build for x86_64"
  fi
fi

# Clean up the temporary directory
rm -rf "$BUILD_DIR"

cat << EOF

Cross-compilation complete.

NOTE: For successful cross-compilation with CGO dependencies (like framebuffer),
you likely need to install the appropriate cross-compilers:

For Debian/Ubuntu:
  sudo apt-get install gcc-arm-linux-gnueabihf gcc-aarch64-linux-gnu

You may also need to install the appropriate development headers for the target platforms:
  sudo apt-get install linux-libc-dev-armhf-cross linux-libc-dev-arm64-cross

For more complex cross-compilation, consider using Docker with images that have
the complete toolchain for your target architectures.
EOF
