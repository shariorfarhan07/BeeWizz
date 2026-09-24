// Package order defines how device rows are grouped and sorted, kept
// pluggable so the ordering rule can change without touching the UI or
// bluetooth packages.
package order

import (
	"strings"

	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
)

// Group is a coarse ordering bucket. Groups sort in ascending order:
// connected devices first, then favorites, then everything else.
type Group int

const (
	GroupConnected Group = iota
	GroupFavorite
	GroupOther
)

// Sorter decides both the group and the within-group order of two
// devices.
type Sorter interface {
	Group(d *bluetooth.BluetoothDevice) Group
	Compare(a, b *bluetooth.BluetoothDevice) int
}

// DefaultSorter implements the spec order: connected first, then
// favorites, then others; alphabetical (case-insensitive) by display
// name within a group, with the address as a tie-breaker. A connected
// favorite counts as GroupConnected.
type DefaultSorter struct {
	IsFavorite func(address string) bool
}

func (s DefaultSorter) Group(d *bluetooth.BluetoothDevice) Group {
	if d == nil {
		return GroupOther
	}
	if d.Connected {
		return GroupConnected
	}
	if s.IsFavorite != nil && s.IsFavorite(d.Address) {
		return GroupFavorite
	}
	return GroupOther
}

func (s DefaultSorter) Compare(a, b *bluetooth.BluetoothDevice) int {
	ga, gb := s.Group(a), s.Group(b)
	if ga != gb {
		if ga < gb {
			return -1
		}
		return 1
	}
	na, nb := strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName())
	if na != nb {
		if na < nb {
			return -1
		}
		return 1
	}
	if a.Address != b.Address {
		if a.Address < b.Address {
			return -1
		}
		return 1
	}
	return 0
}
