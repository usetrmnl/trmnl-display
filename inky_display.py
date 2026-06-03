#!/usr/bin/env python3
"""Render an image on a Pimoroni Inky display for trmnl-display.

This is an alternative rendering backend to the bb_epaper `show_img` binary.
It drives any board supported by Pimoroni's `inky` library, including the
Inky Impression 13.3" (Spectra 6, EL133UF1, 1600x1200) which bb_epaper does
not yet support.

It accepts the same `key=value` argument convention as `show_img` so that
trmnl-display can call either backend interchangeably:

    inky_display.py file=/path/img.png [invert=true|false] [mode=...] [saturation=0.5]

Arguments:
    file        path to the image to display (PNG/JPEG/BMP - anything Pillow opens)
    invert      "true" to invert the image (dark mode); anything else = normal
    mode        accepted for compatibility and ignored - Inky Spectra/Impression
                panels only support full refreshes
    saturation  colour saturation 0.0-1.0 for Impression/Spectra panels (default 0.5)

Smoke test:
    inky_display.py selftest [saturation=0.5] [out=preview.png] [size=WxH]

    Generates a panel-sized Spectra-6 test pattern (six colour bars, a 1px
    border, a centre seam line and corner-to-corner diagonals) and displays it.
    A kink in the diagonals at the centre, or a broken centre line, reveals a
    left/right half misalignment on the dual-controller 13.3" panel. With no
    board attached, pass `size=1600x1200 out=preview.png` to just write the
    pattern to a PNG you can inspect on any machine.
"""
import sys


def parse_args(argv):
    opts = {}
    for arg in argv:
        if "=" in arg:
            key, value = arg.split("=", 1)
            opts[key.strip()] = value.strip()
    return opts


def init_inky():
    """Auto-detect the attached Inky board via its EEPROM. Returns the board
    object, or raises on failure. auto()'s signature varies across releases."""
    from inky.auto import auto
    try:
        return auto(ask_user=False, verbose=False)
    except TypeError:
        return auto(ask_user=False)


def show(inky, img, saturation):
    """Push a PIL image to the panel. Colour Impression/Spectra panels take a
    `saturation` argument; mono/red/yellow boards (pHAT/wHAT) do not."""
    try:
        inky.set_image(img, saturation=saturation)
    except TypeError:
        inky.set_image(img)
    inky.show()


def _load_font(size):
    from PIL import ImageFont
    for path in (
        "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    ):
        try:
            return ImageFont.truetype(path, size)
        except Exception:
            pass
    try:  # Pillow >= 10 lets the built-in font be scaled
        return ImageFont.load_default(size=size)
    except TypeError:
        return ImageFont.load_default()


def _label(draw, xy, text, font):
    """Draw text on a white background so it stays legible over colour bars."""
    x, y = xy
    try:
        l, t, r, b = draw.textbbox((x, y), text, font=font)
        draw.rectangle([l - 4, t - 4, r + 4, b + 4], fill=(255, 255, 255))
    except Exception:
        pass
    draw.text((x, y), text, fill=(0, 0, 0), font=font)


def build_test_image(width, height):
    """Build a Spectra-6 smoke-test pattern at the given resolution."""
    from PIL import Image, ImageDraw

    colors = [
        (0, 0, 0),        # black
        (255, 255, 255),  # white
        (255, 0, 0),      # red
        (0, 255, 0),      # green
        (0, 0, 255),      # blue
        (255, 255, 0),    # yellow
    ]

    img = Image.new("RGB", (width, height), (255, 255, 255))
    draw = ImageDraw.Draw(img)

    # Six vertical colour bars across the full width.
    bar_w = width / len(colors)
    for i, rgb in enumerate(colors):
        draw.rectangle([int(i * bar_w), 0, int((i + 1) * bar_w), height], fill=rgb)

    # 1px border to verify edges and orientation/rotation.
    draw.rectangle([0, 0, width - 1, height - 1], outline=(0, 0, 0), width=2)

    # Centre seam: dual-controller panels (13.3") split here. A break in this
    # line means the two halves are misaligned.
    cx = width // 2
    draw.line([cx, 0, cx, height], fill=(0, 0, 0), width=3)

    # Full-width diagonals: a kink at the centre also reveals a half offset.
    draw.line([0, 0, width - 1, height - 1], fill=(0, 0, 0), width=2)
    draw.line([0, height - 1, width - 1, 0], fill=(0, 0, 0), width=2)

    font = _load_font(max(16, height // 40))
    _label(draw, (10, 10), f"{width}x{height} Spectra6 selftest", font)
    _label(draw, (10, height // 2), "LEFT", font)
    _label(draw, (cx + 12, height // 2), "RIGHT", font)
    _label(draw, (cx + 12, 10), "<- centre seam", font)
    return img


def run_selftest(opts):
    try:
        from PIL import Image  # noqa: F401  (ensure Pillow is importable early)
    except ImportError as exc:
        print(f"inky_display: Pillow is not installed: {exc}", file=sys.stderr)
        return 1

    try:
        saturation = float(opts.get("saturation", "0.5"))
    except ValueError:
        saturation = 0.5

    # Resolution: prefer the attached board; fall back to an explicit size= for
    # off-device previewing.
    inky = None
    try:
        inky = init_inky()
        width, height = inky.resolution
    except Exception as exc:
        if "size" in opts:
            try:
                width, height = (int(v) for v in opts["size"].lower().split("x", 1))
            except Exception:
                print(f"inky_display: invalid size '{opts['size']}' (expected WxH)", file=sys.stderr)
                return 1
            print(f"inky_display: no Inky board detected ({exc}); generating preview only", file=sys.stderr)
        else:
            print(f"inky_display: could not initialise Inky display: {exc}", file=sys.stderr)
            print("inky_display: pass size=WxH out=preview.png to generate a preview without a board", file=sys.stderr)
            return 1

    img = build_test_image(width, height)

    out = opts.get("out")
    if out:
        try:
            img.save(out)
            print(f"inky_display: wrote test pattern to {out} ({width}x{height})")
        except Exception as exc:
            print(f"inky_display: could not save {out}: {exc}", file=sys.stderr)
            return 1

    if inky is not None:
        print(f"inky_display: displaying {width}x{height} test pattern (full refresh)...")
        show(inky, img, saturation)
        print("inky_display: selftest refresh complete")
    return 0


def render_file(opts):
    try:
        from PIL import Image, ImageOps
    except ImportError as exc:
        print(f"inky_display: Pillow is not installed: {exc}", file=sys.stderr)
        return 1

    path = opts.get("file")
    if not path:
        print("inky_display: missing required 'file=' argument", file=sys.stderr)
        return 1

    invert = opts.get("invert", "false").lower() == "true"
    try:
        saturation = float(opts.get("saturation", "0.5"))
    except ValueError:
        saturation = 0.5

    try:
        inky = init_inky()
    except Exception as exc:
        print(f"inky_display: could not initialise Inky display: {exc}", file=sys.stderr)
        return 1

    try:
        img = Image.open(path)
    except Exception as exc:
        print(f"inky_display: could not open image {path}: {exc}", file=sys.stderr)
        return 1

    img = img.convert("RGB")
    if invert:
        img = ImageOps.invert(img)

    # Fit the image to the panel while preserving aspect ratio, padding with
    # white if the aspect ratios differ. When the server already renders at the
    # panel's native resolution (the expected case) this is a no-op.
    if tuple(img.size) != tuple(inky.resolution):
        img = ImageOps.pad(img, inky.resolution, color=(255, 255, 255))

    show(inky, img, saturation)
    return 0


def main():
    argv = sys.argv[1:]
    opts = parse_args(argv)
    if "selftest" in argv or opts.get("test", "").lower() == "true":
        return run_selftest(opts)
    return render_file(opts)


if __name__ == "__main__":
    sys.exit(main())
