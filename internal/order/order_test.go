package order

import (
	"testing"

	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
)

func dev(name, addr string, connected bool) *bluetooth.BluetoothDevice {
	return &bluetooth.BluetoothDevice{Alias: name, Address: addr, Connected: connected}
}

func TestDefaultSorter_ConnectedFirst(t *testing.T) {
	s := DefaultSorter{IsFavorite: func(string) bool { return false }}
	a := dev("Zebra", "AA", true)
	b := dev("Alpha", "BB", false)
	if s.Compare(a, b) >= 0 {
		t.Fatalf("connected device should sort before disconnected regardless of name")
	}
}

func TestDefaultSorter_FavoritesNextThenOthers(t *testing.T) {
	favs := map[string]bool{"FA": true}
	s := DefaultSorter{IsFavorite: func(a string) bool { return favs[a] }}
	fav := dev("Fav", "FA", false)
	other := dev("Other", "OT", false)
	if s.Compare(fav, other) >= 0 {
		t.Fatalf("favorite should sort before other")
	}
}

func TestDefaultSorter_AlphabeticalCaseInsensitive(t *testing.T) {
	s := DefaultSorter{IsFavorite: func(string) bool { return false }}
	a := dev("apple", "AA", false)
	b := dev("Banana", "BB", false)
	if s.Compare(a, b) >= 0 {
		t.Fatalf("'apple' should sort before 'Banana' case-insensitively")
	}
}

func TestDefaultSorter_AddressTieBreaker(t *testing.T) {
	s := DefaultSorter{IsFavorite: func(string) bool { return false }}
	a := dev("Same", "AA:AA", false)
	b := dev("Same", "BB:BB", false)
	if s.Compare(a, b) >= 0 {
		t.Fatalf("expected address AA:AA to sort before BB:BB")
	}
}

func TestDefaultSorter_ConnectedFavoriteIsConnectedGroup(t *testing.T) {
	favs := map[string]bool{"FA": true}
	s := DefaultSorter{IsFavorite: func(a string) bool { return favs[a] }}
	connFav := dev("X", "FA", true)
	if s.Group(connFav) != GroupConnected {
		t.Fatalf("connected favorite should be in GroupConnected, got %v", s.Group(connFav))
	}
}
