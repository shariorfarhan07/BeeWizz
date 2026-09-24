package main

import (
	"log/slog"

	"github.com/shariorfarhan/bluetooth-widget/internal/audio"
	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
	"github.com/shariorfarhan/bluetooth-widget/internal/config"
	"github.com/shariorfarhan/bluetooth-widget/internal/ui"
)

// runUI launches the GTK4 window.
func runUI(cfgStore *config.Store) int {
	m := bluetooth.NewSystemManager(slog.Default())
	return ui.Run(ui.Options{
		Manager: m,
		Audio:   audio.NewPulseController(slog.Default()),
		Config:  cfgStore,
		Logger:  slog.Default(),
		Version: version,
	})
}
