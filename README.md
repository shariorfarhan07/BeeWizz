# 🎧 Bluetooth Widget

**A native Linux desktop widget for Bluetooth device management — connect, disconnect, and configure audio with one click. No `bluetoothctl`, no root, no shell scripts.**

[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://golang.org)
[![GTK](https://img.shields.io/badge/GTK-4.6%2B-009EDA)](https://www.gtk.org)
[![License](https://img.shields.io/badge/License-MIT-blue)](LICENSE)
[![Build](https://img.shields.io/badge/Status-Stable-brightgreen)]()

## ✨ Why Bluetooth Widget?

Linux desktops make it surprisingly hard to manage Bluetooth audio. You need **five clicks** in GNOME Settings to switch between audio modes. Bluetooth Widget gives you **one click**—and it's always visible.

- 🎯 **One-click connect/disconnect** — no modal dialogs
- 🔊 **Audio mode switcher** — toggle between high-quality playback (A2DP) and headset mode (HFP) instantly
- 🎚️ **Codec picker** — switch between SBC, SBC XQ, aptX, LDAC on PulseAudio >= 15
- 🔋 **Battery levels** — shows charge % for each earbud, case, controller
- ⚡ **Real-time updates** — changes from GNOME Settings appear instantly, no polling
- ⭐ **Favorites** — pin your daily drivers to the top
- 🪟 **Always-on-top** (X11) — stays visible while you work
- 🚀 **No dependencies** — native GTK4, pure Go, talks directly to BlueZ over D-Bus

```
Before:  Settings → Bluetooth → ⌛ wait → click device → Audio settings... → Audio mode dropdown
Now:     Click speaker icon → Audio mode → Done
```

## 🎮 Quick Start

### Build & Run
```bash
git clone https://github.com/yourname/bluetooth-widget && cd bluetooth-widget
make build
make run
```

### Install for your user (no root)
```bash
./scripts/install.sh --autostart
```

The widget appears in your app menu and auto-launches on login.

## 📸 Interface

```
┌─────────────────────────────────────┐
│ Bluetooth                       ⚙   │
├─────────────────────────────────────┤
│ CONNECTED                           │
│                                     │
│ 🎧 CMF Buds Pro 2         🔊 ⭐    │
│     ● Connected           82%      │
│                                     │
│ PAIRED                              │
│                                     │
│ 🖱️  Logitech MX Master    🔊 ⭐    │
│     ○ Click to connect             │
│                                     │
│ ⌨️  Keychron K2                 ⭐  │
│     ○ Click to connect             │
│                                     │
└─────────────────────────────────────┘
```

**Click the 🔊 icon on any connected device:**
- **Audio Mode**: High quality (A2DP) ↔ Headset (HFP)
- **Output Device**: Pick system default speaker
- **Input Device**: Pick system default microphone (auto-switches to HFP if needed)
- **Codec**: Switch A2DP codecs (PulseAudio >= 15 only)

## 🏗️ Architecture

```
User clicks device → GTK4 UI → BluetoothManager (pure Go)
                                    ↓
                              D-Bus (native protocol)
                                    ↓
                              BlueZ (bluetoothd)
                              
Audio settings         → PulseAudio Controller (pure Go)
                              ↓
                         D-Bus (native protocol)
                              ↓
                         PulseAudio (or pipewire-pulse)
```

**Why pure Go?**
- No C string parsing (`bluetoothctl` output is fragile)
- No shell injection risks
- No subprocess overhead
- Type-safe D-Bus and protobuf handling
- Easy to mock and test

**Key packages:**
- `internal/bluetooth/` — D-Bus layer + BlueZ state machine (testable; mocked D-Bus)
- `internal/audio/` — PulseAudio native protocol (codecs, profiles, defaults)
- `internal/ui/` — GTK4 frontend (only package using cgo)
- `internal/cli/` — headless modes (`-dump`, `-watch`, `-connect`, etc.)

## 🔧 Features Deep-Dive

### One-Click Connect/Disconnect
Clicking a device row connects or disconnects it. A spinner shows while the operation is in flight. Failed attempts show a brief error with a Retry button.

### Real-Time Updates
Device changes from GNOME Settings, `bluetoothctl`, or other apps appear instantly. No polling loop; the widget subscribes to BlueZ D-Bus signals:
- `PropertiesChanged` — device connection status, battery %
- `InterfacesAdded` / `InterfacesRemoved` — new/lost devices
- `NameOwnerChanged` — bluetoothd restart

### Audio Settings (Connected Devices)
**Requires:** PulseAudio >= 15 or PipeWire with pipewire-pulse

- **Audio Mode**
  - *High Quality (A2DP)*: Best sound, no microphone
  - *Headset (HFP)*: Microphone available, reduced quality
  - *Off*: Device disconnected from audio

- **Output / Input Device**
  - Lists all system outputs/inputs (speakers, headphones, mics, etc.)
  - Clicking a device makes it the system default
  - Picking the device's own microphone auto-switches to headset mode

- **Codec** (PulseAudio >= 15 only)
  - Shows codecs both device and system support
  - Switches between SBC, SBC XQ, aptX, LDAC, AAC, etc.
  - Only works in high-quality (A2DP) mode
  - PipeWire encodes codec in profile name instead

### Battery & Icons
- Displays charge % from BlueZ `Battery1` interface
- Supports multi-battery devices (L/R/Case for earbuds)
- Device icons from freedesktop.org, mapped from BlueZ `Icon` property
- Generic Bluetooth icon fallback

### Persistent Settings
- Window size (and position, on X11)
- Favorite devices (pinned to top)
- Preferences (always-on-top, show battery, autostart, etc.)
- Stored in `~/.config/bluetooth-widget/config.json`

## 📋 Requirements

**System:**
- Linux with BlueZ >= 5.50 and D-Bus
- GTK4 >= 4.6 (runtime)

**Build:**
- Go >= 1.22
- GCC + pkg-config
- GTK4 development files:
  ```bash
  sudo apt install build-essential pkg-config libgtk-4-dev libgirepository1.0-dev
  ```

**Audio (optional):**
- PulseAudio >= 15 (for codec switching) or PipeWire with pipewire-pulse

## 🚀 Installation

### From Source
```bash
git clone https://github.com/yourname/bluetooth-widget
cd bluetooth-widget
make build
./scripts/install.sh --autostart
```

### Run Without Installing
```bash
make run
```

### Available Make Targets
```bash
make build          # Compile to ./bluetooth-widget
make run            # Build and run
make test           # Run all tests
make test-race      # Run with race detector
make test-integration  # Read-only tests against real BlueZ
make vet            # Run go vet
make install        # Install to ~/.local (user-local)
make uninstall      # Remove from ~/.local
make clean          # Delete build artifacts
```

## 📖 CLI Modes

The widget also works headless:

```bash
# List all devices as JSON
./bluetooth-widget -dump

# Watch for Bluetooth changes
./bluetooth-widget -watch

# Connect/disconnect by MAC
./bluetooth-widget -connect 2C:BE:EE:79:AC:64
./bluetooth-widget -disconnect 2C:BE:EE:79:AC:64

# Check audio state (profiles, codecs, outputs, inputs)
./bluetooth-widget -audio

# Turn Bluetooth on/off
./bluetooth-widget -power on
./bluetooth-widget -power off

# Enable/disable autostart
./bluetooth-widget -autostart on
./bluetooth-widget -autostart off

# Verbose logging
./bluetooth-widget -debug

# Print version
./bluetooth-widget -version
```

## 🔐 Security

- ✅ Runs as normal user — no root
- ✅ Never shells out to `bluetoothctl`, `rfkill`, `hciconfig`
- ✅ Never parses command output
- ✅ Never modifies system Bluetooth config
- ✅ Never stores Bluetooth pairing keys
- ✅ D-Bus communication over system bus only

## 🧪 Testing

```bash
# All tests (pure Go only)
make test

# With race detector (pure Go packages)
make test-race

# Integration tests (read-only, against real BlueZ)
# Requires a running bluetoothd
make test-integration

# Unit test coverage
make vet
```

The D-Bus layer is fully mocked in tests via a `bus` interface, so tests run without hardware.

## 🐛 Troubleshooting

| Problem | Solution |
|---------|----------|
| **No Bluetooth devices shown** | Ensure BlueZ is running: `systemctl status bluetooth` |
| **"Not in range" for all devices** | Check Bluetooth adapter: `rfkill list` — may be soft-blocked |
| **Audio settings button not visible** | Device must be connected AND have an audio card in PulseAudio (check with `-audio` flag) |
| **Can't switch codecs** | PulseAudio must be >= 15; PipeWire puts codec in audio mode list instead |
| **GTK build errors** | Install dev packages: `sudo apt install libgtk-4-dev libgirepository1.0-dev` |
| **Connection fails with "In use"** | Device may be locked by another app (e.g., existing Bluetooth connection) |

## 📁 Project Structure

```
bluetooth-widget/
├── cmd/bluetooth-widget/    # main(), flag parsing, dispatch
├── internal/
│   ├── bluetooth/           # Pure Go: D-Bus, BlueZ, state machine
│   ├── audio/               # Pure Go: PulseAudio native protocol
│   ├── order/               # Device sort order (pluggable)
│   ├── config/              # JSON config store (~/.config)
│   ├── autostart/           # ~/.config/autostart management
│   ├── logging/             # Structured logging (slog)
│   ├── cli/                 # -dump, -watch, -connect, etc.
│   └── ui/                  # GTK4 UI (only cgo package)
│       └── x11win/          # X11 window hints (cgo), nox11 stub
├── third_party/pulse/       # Vendored jfreymuth/pulse proto + extensions
├── data/                    # .desktop file, icons
├── scripts/                 # install.sh, uninstall.sh, run.sh
├── tests/integration/       # Optional real-BlueZ tests
├── Makefile
├── go.mod / go.sum
└── README.md
```

## 🛠️ Development

### Adding a Feature

1. **Core logic** → `internal/bluetooth/` or `internal/audio/`
   - Write pure Go first
   - Add unit tests with fake D-Bus/PulseAudio server
   - Run with `-race` flag

2. **UI** → `internal/ui/`
   - Use GTK4 widgets from gotk4 v0.2.2
   - Call core logic off the main thread
   - Return results via `glib.IdleAdd`

3. **Test**
   ```bash
   make test-race
   make vet
   ```

### D-Bus API Reference

BlueZ: https://github.com/bluez/bluez/tree/master/doc
PulseAudio: https://www.freedesktop.org/wiki/Software/PulseAudio/Documentation/

### Debugging

```bash
# See D-Bus messages
DBUS_VERBOSE=1 ./bluetooth-widget -debug

# Watch BlueZ signals
busctl monitor org.bluez

# List available audio cards
pactl list cards
```

## 📜 License

MIT — see [LICENSE](LICENSE)

## 🙏 Acknowledgments

- [BlueZ](http://www.bluez.org/) — Linux Bluetooth stack
- [gotk4](https://github.com/diamondburned/gotk4) — Go GTK4 bindings
- [godbus](https://github.com/godbus/dbus) — Go D-Bus library
- [PulseAudio](https://www.freedesktop.org/wiki/Software/PulseAudio/) — Linux sound server

---

**Made with ❤️ for Linux desktop users who actually use Bluetooth.** PRs and issues welcome!
