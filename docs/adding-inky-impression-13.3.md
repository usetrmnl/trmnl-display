# Adding support for the Pimoroni Inky Impression 13.3" (EL133UF1)

> **Implemented route:** the project now drives the 13.3" via Pimoroni's `inky`
> Python library (build.sh option 4, [`inky_display.py`](../inky_display.py)),
> which already supports this panel. That avoids all of the bb_epaper driver
> work described below. **This document is kept as reference** for anyone who
> later wants to add native EL133UF1 support to bb_epaper itself (e.g. to keep a
> single C backend for every display).

This is an implementation plan / patch spec for adding the **Pimoroni Inky
Impression 13.3"** (and the equivalent **Waveshare 13.3" e‑Paper HAT+ (E)**) to
the TRMNL display pipeline **via bb_epaper**. The 7.3" Inky already works; this
document explains exactly what is missing and how to close the gap.

> **Status:** spec only. No code in this repo makes the 13.3" work on its own —
> the bulk of the work is upstream in `bitbank2/bb_epaper`. See
> [§4 Decision: where the work lives](#4-decision-where-the-work-lives).

---

## 1. Panel facts

| Property            | Value                                                       |
|---------------------|-------------------------------------------------------------|
| Display             | Pimoroni Inky Impression 13.3" / Waveshare 13.3" HAT+ (E)   |
| Panel / driver IC   | **E Ink Spectra 6 — EL133UF1** (a controller in its own right, *not* UC81xx) |
| Resolution          | **1600 × 1200**                                             |
| Colours             | 6 (black, white, yellow, red, blue, green) — Spectra 6      |
| Bus                 | SPI, **two chip-selects** (M / S controllers)               |
| Geometry            | Two controllers, each driving **1200 × 800** (TRES = `04 B0 03 20`). The 1600-wide image is split into two 800-wide halves. |
| Full refresh        | ~32 s (long busy-wait after `DRF`)                          |

Reference drivers for the init sequence:
- Pimoroni `inky` — `inky/inky_el133uf1.py` (used to derive the sequence below).
- Waveshare 13.3" e‑Paper HAT+ (E) demo code.

---

## 2. How the pipeline works today (why this repo alone can't do it)

```
trmnl-display.go  ──exec──▶  show_img file=… invert=… mode=…
build.sh          ──writes─▶ ~/.config/trmnl/show_img.json
                              { adapter, panel_1bit, panel_2bit, stretch }
                                          │
                                          ▼
   bb_epaper (cloned + built by build.sh at install time)
   ├─ rpi/examples/show_img/main.cpp   name→enum lookup (szPanels[]),  GPIO table (adapters[])
   └─ src/bb_ep.inl                    panelDefs[]  {w,h,init-seq,flags,chip,colour-table}
```

`build.sh` only ever selects an **adapter string** + a **panel-name string**.
The panel must already be defined in `bb_epaper`. The 7.3" works because:

- `build.sh:64-66` → `adapter=pimoroni`, `panel=EP73_SPECTRA_800x480`
- `bb_ep.inl` defines that panel: `{800,480,0, epd73_spectra_init, …, BBEP_7COLOR, BBEP_CHIP_UC81xx, u8Colors_spectra}`

There is **no 1600×1200 / EL133 / 13.3" entry anywhere in the current
`bb_epaper`** (largest panel is `EP1085_1360x480`). Adding a `build.sh` menu
option that points at a nonexistent panel name makes `show_img` abort with
`Invalid 1-bit panel`.

The good news: the library already has the *infrastructure* a dual-controller
Spectra 6 panel needs — `BBEP_SPLIT_BUFFER` (0x0400), `BBEP_7COLOR` (0x0020),
the `u8Colors_spectra` colour table, the dual-CS plumbing
(`setCS2` / `bbepSetCS2`, with an RPi implementation in `rpi_io.inl`), and
`bbepWriteImage4bppDual()`. The existing **8.1" `EP81_SPECTRA_1024x576`
("dual cable Spectra 6")** is the closest template.

### 2.1 The one thing the existing mechanism can't express

The 8.1" panel sends the **same** flat init array to both controllers (CS
toggled). The EL133UF1 needs **per-controller** init commands:

- `ANTM`, `DCDC`, `CMDA4`, `PWR`, `EN_BUF`, `BTST_*`, `BOOST_*`, `TFT_VCOM` → **CS0 only**
- `POFS` → **different data per controller**: CS0 = `00 C0 03 A8`, CS1 = `00 C0 03 9A`
- `CMD66`, `PSR`, `PLL`, `CDI`, `TCON`, `AGID`, `PWS`, `CCSET`, `TRES`, `DTM`, `PON`, `DRF`, `POF` → **both**

So the init-sequence format (a single flat byte array in `panelDefs[]`) must be
extended to carry a per-command CS target, *or* the EL133 needs its own init
function rather than the table-driven path. This is the core of the upstream work.

---

## 3. Patch spec

### 3.1 Upstream — `bitbank2/bb_epaper` (the substantive work)

**(a) `src/bb_epaper.h` — new chip type.** EL133UF1 is not UC81xx; add a 5th
chip enum (insert before `BBEP_CHIP_COUNT`):

```c
enum {
    BBEP_CHIP_NOT_DEFINED = 0,
    BBEP_CHIP_SSD16xx,
    BBEP_CHIP_UC81xx,
    BBEP_CHIP_IT8951,
    BBEP_CHIP_EL133,     // <-- NEW: E Ink Spectra 6 dual-controller (1600x1200)
    BBEP_CHIP_NONE,
    BBEP_CHIP_COUNT
};
```

**(b) `src/bb_epaper.h` — new panel enum.** Append to the **end** of the panel
type enum (the comment in `bb_ep.inl` says *"ONLY ADD NEW PANELS TO THE END"* —
order must track `panelDefs[]`):

```c
    EP133_SPECTRA_1600x1200, // EL133UF1 13.3" Spectra 6, dual controller
```

**(c) `src/bb_ep.inl` — command constants + init data.** Define the EL133UF1
command set and the (CS-tagged) init sequence. Command bytes (from the Pimoroni
reference):

```c
#define EL133_ANTM   0x74
#define EL133_CMD66  0xF0
#define EL133_PSR    0x00
#define EL133_DCDC   0xA5
#define EL133_PLL    0x30
#define EL133_CDI    0x50
#define EL133_TCON   0x60
#define EL133_POFS   0x03
#define EL133_AGID   0x86
#define EL133_PWS    0xE3
#define EL133_CCSET  0xE0
#define EL133_TRES   0x61
#define EL133_CMDA4  0xA4
#define EL133_PWR    0x01
#define EL133_EN_BUF 0xB6
#define EL133_BTST_P 0x06
#define EL133_VDDP   0xB7
#define EL133_BTST_N 0x05
#define EL133_VDDN   0xB0
#define EL133_VCOM   0xB1
#define EL133_DTM    0x10
#define EL133_PON    0x04
#define EL133_DRF    0x12
#define EL133_POF    0x02
```

Full init sequence, in order (CS target in the third column):

| Cmd  | Const     | CS   | Data                          |
|------|-----------|------|-------------------------------|
| 0x74 | ANTM      | CS0  | `00 0C 0C D9 DD DD 15 15 55`  |
| 0xF0 | CMD66     | both | `49 55 13 5D 05 10`           |
| 0x00 | PSR       | both | `DF 6B`                       |
| 0xA5 | DCDC      | CS0  | `44 54 00`                    |
| 0x30 | PLL       | both | `08`                          |
| 0x50 | CDI       | both | `37`                          |
| 0x60 | TCON      | both | `03 03`                       |
| 0x03 | POFS      | CS0  | `00 C0 03 A8`                 |
| 0x03 | POFS      | CS1  | `00 C0 03 9A`                 |
| 0x86 | AGID      | both | `10`                          |
| 0xE3 | PWS       | both | `22`                          |
| 0xE0 | CCSET     | both | `01`                          |
| 0x61 | TRES      | both | `04 B0 03 20`  (1200 × 800)   |
| 0xA4 | CMDA4     | CS0  | `03 00 01 03 00 03 00 00 00`  |
| 0x01 | PWR       | CS0  | `0F 00 28 2C 28 38`           |
| 0xB6 | EN_BUF    | CS0  | `07`                          |
| 0x06 | BTST_P    | CS0  | `E0 20`                       |
| 0xB7 | VDDP      | CS0  | `01`                          |
| 0x05 | BTST_N    | CS0  | `E0 20`                       |
| 0xB0 | VDDN      | CS0  | `01`                          |
| 0xB1 | VCOM      | CS0  | `02`                          |

Update / refresh path (used by `bbepRefresh` for this chip):

1. `DTM` (0x10) image data → CS0 (left 800) and CS1 (right 800) separately.
2. `PON` (0x04) → both; busy-wait.
3. `DRF` (0x12) data `00` → both; **busy-wait ~32 s** (give it a generous timeout).
4. `POF` (0x02) data `00` → both; busy-wait, then `sleep(DEEP_SLEEP)`.

Because of the per-CS commands and the EL133-specific refresh, the cleanest
implementation is a dedicated `BBEP_CHIP_EL133` code path in
`bbepSendInitSequence`/`bbepRefresh` (switch on `chip_type`) rather than forcing
the existing table format to carry CS tags. If you prefer the table-driven
route, extend the init-array opcode set with a `SET_CS <0|1|both>` pseudo-opcode
that the sequence interpreter honours.

**(d) `src/bb_ep.inl` — `panelDefs[]` row.** Append at the end (order must match
the enum from step (b)):

```c
    {1200, 800, 0, epd133_el133_init, NULL, NULL,
     BBEP_SPLIT_BUFFER | BBEP_7COLOR, BBEP_CHIP_EL133, u8Colors_spectra},
     // EP133_SPECTRA_1600x1200  (each controller 1200x800; combined 1600x1200)
```

> Note the dimensions: store per-controller geometry consistent with how
> `BBEP_SPLIT_BUFFER` is consumed elsewhere (the 8.1" stores the *combined*
> 1024×576 and the split logic halves it). Match whichever convention
> `bbepWriteImage4bppDual` + `width()/height()` already assume — verify against
> the 8.1" path and mirror it. The combined surface must end up 1600×1200.

**(e) `rpi/examples/show_img/main.cpp` — name lookup.** Append to `szPanels[]`
(it currently ends at index 64, `EP75YR_800x480`); the index must match the new
enum position:

```c
    "EP75YR_800x480", "EP133_SPECTRA_1600x1200", // 64-65
    NULL // must be last entry
```

**(f) `rpi/examples/show_img/main.cpp` — adapter + second CS.** The 13.3" HAT
needs a second chip-select wired and `setCS2()` called. Per the Pimoroni 13.3"
reference the BCM pins are:

| Signal | BCM |
|--------|-----|
| DC     | 22  |
| RST    | 27  |
| BUSY   | 17  |
| CS0    | 26  |
| CS1    | 16  |
| PWR    | (none / 0xff) |

The existing `ADAPTER` struct (`{u8DC,u8RST,u8BUSY,u8CS,u8PWR,u8SPI}`) has **no
field for a 2nd CS**. Add one (`u8CS2`) and a new adapter entry, e.g.
`"inky_13"`, then after `initIO(...)` call `bbep.setCS2(cs2)` whenever the panel
has `BBEP_SPLIT_BUFFER`:

```c
// szAdapters[] add: "inky_13"
// adapters[] add:   { 22, 27, 17, 26, 0xff, 0, /*u8CS2=*/16 }
...
bbep.initIO(a.u8DC, a.u8RST, a.u8BUSY, a.u8CS, a.u8SPI, 0, 8000000);
if (bbep.capabilities() & BBEP_SPLIT_BUFFER) {
    bbep.setCS2(a.u8CS2);   // <-- show_img never calls this today; required for true dual-controller
}
```

> ⚠️ `show_img` does **not** currently call `setCS2()` for any panel. The 8.1"
> dual-cable panel apparently works without it; the EL133UF1 has two independent
> controllers and genuinely needs both CS lines driven. This wiring is new work.

> ⚠️ On Raspberry Pi, CS0=GPIO26/CS1=GPIO16 are software-driven GPIOs, so the
> kernel SPI CS must be freed: add `dtoverlay=spi0-0cs` to
> `/boot/firmware/config.txt` (matches Pimoroni's own guidance). `build.sh`
> should apply this for the 13.3" option (see 3.2).

**(g) RAM / buffer.** 1600×1200 at 4 bpp ≈ **960 KB** for the working buffer
(plus the split halves). Fine on a Pi; just confirm `allocBuffer()` sizing and
the line cache (`u8Cache[512]`) are adequate for a 1200-wide controller half.

### 3.2 This repo — `trmnl-display`

Trivial once the panel constant exists upstream.

**`build.sh`** — add a 4th menu option:

```sh
  echo "  4) Pimoroni Inky Impression Spectra 13.3"
  ...
  case $n in
    ...
    4) JADAPTER="inky_13"
       echo "dtoverlay=spi0-0cs" | sudo tee -a /boot/firmware/config.txt   # free CS0/CS1 for GPIO use
       PANEL2="EP133_SPECTRA_1600x1200"
       PANEL="EP133_SPECTRA_1600x1200";;
```

(If the upstream adapter is named `pimoroni` rather than a new `inky_13`, use
that instead. The `spi0-0cs` overlay line is only needed if CS pins are
software-driven — confirm against the final adapter wiring.)

**`README.md`** — add the 13.3" to the supported-display list (currently lists
the 7.3" at the "Pimoroni Inky Impression Spectra 7.3"" bullet).

### 3.3 Upstream dependency pinning

`build.sh` clones `bb_epaper` from `master`. Until bitbank2 merges EL133UF1
support, the 13.3" option must point `build.sh`'s clone at a fork/branch that
contains the changes from §3.1, then switch back to upstream once merged.

---

## 4. Decision: where the work lives

| Layer | Effort | In this repo? |
|-------|--------|---------------|
| EL133UF1 driver (new chip type, per-CS init, split refresh) | **High** — real driver porting + hardware testing | ❌ upstream `bb_epaper` |
| `show_img` name lookup + dual-CS adapter | Low–Med | ❌ upstream `bb_epaper` (examples) |
| `build.sh` menu option + README | Trivial | ✅ here |

Recommended path: **fork `bb_epaper`**, implement §3.1 against the EL133UF1
reference drivers, validate on real hardware (the 32 s refresh and the per-CS
`POFS` split are the riskiest bits), then submit upstream and land §3.2 here
pointing at the fork until the PR merges.

---

## 5. Verification checklist

- [ ] `show_img panel_1bit=EP133_SPECTRA_1600x1200 adapter=inky_13 file=test.png` runs without "Invalid panel".
- [ ] A full-colour 1600×1200 PNG renders with all 6 Spectra colours and **no left/right half misalignment** (validates the CS split + per-CS `POFS`).
- [ ] Panel is put to sleep / powered off after each refresh (EL133 must not sit in high-voltage state — Waveshare explicitly warns of membrane damage).
- [ ] `trmnl-display` end-to-end: image fetched → converted → displayed on the 13.3".
- [ ] Both Pimoroni (EEPROM-detected) and bare Waveshare HAT+ (E) wiring confirmed.
