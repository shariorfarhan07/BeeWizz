#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

AUTOSTART=0
PREFIX="${HOME}/.local"

while [ $# -gt 0 ]; do
  case "$1" in
    --autostart)
      AUTOSTART=1
      shift
      ;;
    --prefix)
      PREFIX="$2"
      shift 2
      ;;
    --prefix=*)
      PREFIX="${1#--prefix=}"
      shift
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

FAIL=0
HINTS=()

check() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo "  [OK] $desc"
    return 0
  else
    echo "  [FAIL] $desc"
    return 1
  fi
}

echo "Checking prerequisites..."

if command -v go >/dev/null 2>&1; then
  GOVER="$(go env GOVERSION 2>/dev/null | sed 's/go//')"
  GOMAJOR="$(echo "$GOVER" | cut -d. -f1)"
  GOMINOR="$(echo "$GOVER" | cut -d. -f2)"
  if [ "${GOMAJOR:-0}" -gt 1 ] || { [ "${GOMAJOR:-0}" -eq 1 ] && [ "${GOMINOR:-0}" -ge 22 ]; }; then
    echo "  [OK] go >= 1.22 (found $GOVER)"
  else
    echo "  [FAIL] go >= 1.22 (found $GOVER)"
    HINTS+=("Install a newer Go from https://go.dev/dl")
    FAIL=1
  fi
else
  echo "  [FAIL] go not found"
  HINTS+=("Install Go from https://go.dev/dl")
  FAIL=1
fi

if ! check "gcc present" command -v gcc; then
  HINTS+=("sudo apt install build-essential")
  FAIL=1
fi
if ! check "pkg-config present" command -v pkg-config; then
  HINTS+=("sudo apt install pkg-config")
  FAIL=1
fi
if ! check "gtk4 >= 4.6 dev files present" pkg-config --exists 'gtk4 >= 4.6'; then
  HINTS+=("sudo apt install libgtk-4-dev")
  FAIL=1
fi
if ! check "gobject-introspection-1.0 dev files present" pkg-config --exists gobject-introspection-1.0; then
  HINTS+=("sudo apt install libgirepository1.0-dev")
  FAIL=1
fi
if ! check "X11 dev files present (optional: always-on-top support)" pkg-config --exists gtk4-x11 x11; then
  echo "  [WARN] building without X11 always-on-top support"
fi
if systemctl is-active --quiet bluetooth 2>/dev/null || busctl --system status org.bluez >/dev/null 2>&1; then
  echo "  [OK] BlueZ appears to be running"
else
  echo "  [WARN] could not confirm bluetoothd is running (this is only a diagnostic, not required to install)"
fi

if [ "$FAIL" -ne 0 ]; then
  echo ""
  echo "Missing prerequisites. On Debian/Ubuntu, try:"
  echo "  sudo apt install build-essential pkg-config libgtk-4-dev libgirepository1.0-dev"
  exit 1
fi

echo ""
echo "Building (the first GTK4/cgo build can take 5-15 minutes)..."
make build

echo "Installing binary to $PREFIX/bin ..."
install -Dm755 bluetooth-widget "$PREFIX/bin/bluetooth-widget"

echo "Installing desktop file..."
install -d "$HOME/.local/share/applications"
sed "s|^Exec=.*|Exec=$PREFIX/bin/bluetooth-widget|" data/com.example.BluetoothWidget.desktop \
  > "$HOME/.local/share/applications/com.example.BluetoothWidget.desktop"
chmod 644 "$HOME/.local/share/applications/com.example.BluetoothWidget.desktop"

echo "Installing icon..."
install -Dm644 data/icons/com.example.BluetoothWidget.svg \
  "$HOME/.local/share/icons/hicolor/scalable/apps/com.example.BluetoothWidget.svg"

update-desktop-database "$HOME/.local/share/applications" 2>/dev/null || true
gtk-update-icon-cache -q -t -f "$HOME/.local/share/icons/hicolor" 2>/dev/null || true

if [ "$AUTOSTART" -eq 1 ]; then
  "$PREFIX/bin/bluetooth-widget" -autostart on
fi

case ":$PATH:" in
  *":$PREFIX/bin:"*) ;;
  *) echo "Note: $PREFIX/bin is not on your PATH." ;;
esac

echo ""
echo "Installed. Launch 'Bluetooth Widget' from the app menu or run: $PREFIX/bin/bluetooth-widget"
