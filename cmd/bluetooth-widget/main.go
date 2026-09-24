// Command bluetooth-widget is a small floating GTK4 widget for
// connecting to and disconnecting from paired Bluetooth devices via
// BlueZ over D-Bus.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/shariorfarhan/bluetooth-widget/internal/audio"
	"github.com/shariorfarhan/bluetooth-widget/internal/bluetooth"
	"github.com/shariorfarhan/bluetooth-widget/internal/cli"
	"github.com/shariorfarhan/bluetooth-widget/internal/config"
	"github.com/shariorfarhan/bluetooth-widget/internal/logging"
)

var version = "dev"

func init() {
	// GTK must be driven from the same OS thread throughout the
	// process's life.
	runtime.LockOSThread()
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		dump         = flag.Bool("dump", false, "print one JSON snapshot of Bluetooth state and exit")
		watch        = flag.Bool("watch", false, "print a JSON line per Bluetooth change event")
		connectAddr  = flag.String("connect", "", "connect to the paired device with this MAC address")
		disconnAddr  = flag.String("disconnect", "", "disconnect the paired device with this MAC address")
		power        = flag.String("power", "", "set adapter power: on|off")
		autostartArg = flag.String("autostart", "", "enable or disable login autostart: on|off")
		debug        = flag.Bool("debug", false, "enable debug logging")
		showVersion  = flag.Bool("version", false, "print version and exit")
		audioDump    = flag.Bool("audio", false, "print one JSON snapshot of Bluetooth audio state (profiles, codecs, outputs, inputs) and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return 0
	}

	logging.Setup(os.Stderr, logging.LevelFromEnv(*debug))

	cfgPath, err := config.DefaultPath()
	if err != nil {
		slog.Error("resolve config path", logging.KeyError, err.Error())
		return 1
	}
	cfgStore, err := config.Load(cfgPath)
	if err != nil {
		slog.Warn("load config", logging.KeyError, err.Error())
	}

	ctx := context.Background()

	switch {
	case *dump:
		m := bluetooth.NewSystemManager(slog.Default())
		return cli.Dump(ctx, m, os.Stdout)

	case *audioDump:
		return cli.AudioDump(ctx, audio.NewPulseController(slog.Default()), os.Stdout)

	case *watch:
		m := bluetooth.NewSystemManager(slog.Default())
		return cli.Watch(ctx, m, os.Stdout)

	case *connectAddr != "":
		m := bluetooth.NewSystemManager(slog.Default())
		return cli.ConnectByAddress(ctx, m, *connectAddr, true, os.Stdout)

	case *disconnAddr != "":
		m := bluetooth.NewSystemManager(slog.Default())
		return cli.ConnectByAddress(ctx, m, *disconnAddr, false, os.Stdout)

	case *power != "":
		on, ok := parseOnOff(*power)
		if !ok {
			fmt.Fprintln(os.Stderr, "-power expects on|off")
			return 2
		}
		m := bluetooth.NewSystemManager(slog.Default())
		return cli.SetPower(ctx, m, on, os.Stdout)

	case *autostartArg != "":
		on, ok := parseOnOff(*autostartArg)
		if !ok {
			fmt.Fprintln(os.Stderr, "-autostart expects on|off")
			return 2
		}
		return cli.SetAutostart(on, cfgStore, os.Stdout)
	}

	uiFlags := 0
	if *debug {
		uiFlags = 1
	}
	if flag.NFlag() == uiFlags && flag.NArg() == 0 {
		return runUI(cfgStore)
	}

	fmt.Fprintln(os.Stderr, "usage: bluetooth-widget [-dump] [-audio] [-watch] [-connect MAC] [-disconnect MAC] [-power on|off] [-autostart on|off] [-debug] [-version]")
	return 2
}

func parseOnOff(s string) (bool, bool) {
	switch s {
	case "on":
		return true, true
	case "off":
		return false, true
	default:
		return false, false
	}
}
