#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

PREFIX="${HOME}/.local"
PURGE=0

while [ $# -gt 0 ]; do
  case "$1" in
    --purge)
      PURGE=1
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

XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"

removed=()

BIN="$PREFIX/bin/bluetooth-widget"
if [ -e "$BIN" ]; then
  rm -f "$BIN"
  removed+=("$BIN")
fi

DESKTOP="$HOME/.local/share/applications/com.example.BluetoothWidget.desktop"
if [ -e "$DESKTOP" ]; then
  rm -f "$DESKTOP"
  removed+=("$DESKTOP")
fi

ICON="$HOME/.local/share/icons/hicolor/scalable/apps/com.example.BluetoothWidget.svg"
if [ -e "$ICON" ]; then
  rm -f "$ICON"
  removed+=("$ICON")
fi

AUTOSTART="$XDG_CONFIG_HOME/autostart/com.example.BluetoothWidget.desktop"
if [ -e "$AUTOSTART" ]; then
  rm -f "$AUTOSTART"
  removed+=("$AUTOSTART")
fi

update-desktop-database "$HOME/.local/share/applications" 2>/dev/null || true
gtk-update-icon-cache -q -t -f "$HOME/.local/share/icons/hicolor" 2>/dev/null || true

if [ "$PURGE" -eq 1 ]; then
  CFGDIR="$XDG_CONFIG_HOME/bluetooth-widget"
  if [ -e "$CFGDIR" ]; then
    rm -rf "$CFGDIR"
    removed+=("$CFGDIR")
  fi
fi

if [ "${#removed[@]}" -eq 0 ]; then
  echo "Nothing to remove."
else
  echo "Removed:"
  for f in "${removed[@]}"; do
    echo "  $f"
  done
fi
