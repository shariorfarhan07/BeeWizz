# Implementation Plan: Linux Bluetooth Desktop Widget (Go + GTK4 + BlueZ)

The project goes in `/home/farhan.exabyting_bKash.com/Documents/Bkash/bluetooth_widget` (this directory is the repo root). The module path is `github.com/shariorfarhan/bluetooth-widget`.

Follow the phases in order. Build and run after every phase. Where this plan gives an exact name, version or command, use it as written.

---

## 0. What was checked on this machine, and the decisions that follow

| Fact | Evidence | What it means for you |
|---|---|---|
| GTK runtime is **4.6.9**. BlueZ is **5.64**. glib is 2.72, pango 1.50. | dpkg, `bluetoothd -v` | Use no GTK API newer than 4.6. |
| **gotk4 `v0.3.x` and `v0.4.x` will NOT compile against GTK 4.6.** They call `C.gtk_file_dialog_new`, `C.gtk_alert_dialog_*`, `C.gtk_graphics_offload_*` and other 4.10–4.14 functions directly. | grep of the module sources | **Pin `github.com/diamondburned/gotk4/pkg v0.2.2`.** It was generated from a GTK older than 4.6 (it has no `gtk_menu_button_set_child` or `gtk_flow_box_append`) and older glib/pango, so every symbol it uses exists on Ubuntu 22.04. **Never run `go get -u` or `@latest` for gotk4.** |
| gotk4 v0.2.2 needs `go4.org/unsafe/assume-no-moving-gc` and `github.com/KarpelesLab/weak v0.1.1` | its go.mod | Both build under Go 1.24 (checked). No special env variables are needed. |
| gotk4's `gio`/`glib` packages use `#cgo pkg-config: gobject-introspection-1.0` | source | You need `libgirepository1.0-dev`, which the user is installing. |
| Use `github.com/godbus/dbus/v5 v5.2.2` | latest, go 1.20 | — |
| `libgtk-4-dev` depends on `libx11-dev`, and `libx11-dev` is already installed (`pkg-config x11` → 1.7.5). `gdk/x11/gdkx.h` ships in `libgtk-4-dev`. | apt-cache depends | The X11 cgo helper is feasible. Also provide a `nox11` build-tag stub as a fallback. |
| Session is X11 (`DISPLAY=:1`, `XDG_SESSION_TYPE=x11`) | env | You can run the GUI for checks. `xprop`, `xwininfo`, `import`, `jq`, `desktop-file-validate`, `busctl` and `gdbus` are all available. |
| Paired devices on hci0 right now: HAVIT SK838BT, CMF Buds Pro 2, LB-M6, soundcore R50i, "RK N80 " (note the trailing space). None are connected, so no Battery1 objects exist yet. The devices also have GATT child objects (`service*/char*`). | `busctl --system tree org.bluez` | Phase 1 output must match this. Trim whitespace in names. Filter out GATT signal noise. |
| Adwaita and Yaru provide every symbolic icon used below, including `audio-headset-symbolic`, `input-*-symbolic`, `battery-level-XX-symbolic`, `starred-symbolic`/`non-starred-symbolic`, `bluetooth-*-symbolic` and `open-menu-symbolic`. `audio-headphones-symbolic` exists only in Yaru. | find | Use a list of icon candidates plus an `IconTheme.HasIcon` fallback. |

Pinned versions (exact):
```
go 1.24
require (
    github.com/godbus/dbus/v5 v5.2.2
    github.com/diamondburned/gotk4/pkg v0.2.2
)
```

gotk4 v0.2.2 import paths:
```go
"github.com/diamondburned/gotk4/pkg/gtk/v4"
"github.com/diamondburned/gotk4/pkg/gdk/v4"
"github.com/diamondburned/gotk4/pkg/gio/v2"
"github.com/diamondburned/gotk4/pkg/glib/v2"          // glib.IdleAdd, glib.TimeoutAdd
coreglib "github.com/diamondburned/gotk4/pkg/core/glib" // coreglib.InternObject, coreglib.NewValue, coreglib.Value
"github.com/diamondburned/gotk4/pkg/pango"            // pango.EllipsizeEnd
```

---

## 1. File list and responsibilities

```
bluetooth_widget/
├── go.mod / go.sum
├── Makefile
├── README.md
├── .gitignore                       # /bluetooth-widget (binary), *.log
├── cmd/bluetooth-widget/main.go     # flag parsing, logging setup, dispatch to cli.* or ui.Run
├── internal/
│   ├── bluetooth/                   # pure Go, NO GTK import
│   │   ├── bus.go                   # narrow `bus` interface + godbus-backed systemBus
│   │   ├── manager.go               # BluetoothManager interface + systemManager impl (signal loop, ops)
│   │   ├── store.go                 # pure state machine: cache + event computation (no locking, no I/O)
│   │   ├── device.go                # BluetoothDevice, DeviceID, parsing, DisplayName, Equal, Clone
│   │   ├── adapter.go               # Adapter, parsing, default-adapter selection
│   │   ├── battery.go               # BatteryLevel, parsing, primary-battery logic
│   │   ├── icons.go                 # BlueZ Icon -> freedesktop icon candidate list
│   │   ├── errors.go                # OpError, ErrorKind, ConvertError, friendly messages, sentinels
│   │   ├── events.go                # Event, EventKind, State
│   │   └── *_test.go                # unit tests + fake bus (fakebus_test.go)
│   ├── order/order.go (+ _test)     # pluggable Sorter (connected > favorites > others > alpha)
│   ├── config/config.go (+ _test)   # JSON config Store, atomic save
│   ├── autostart/autostart.go (+ _test) # ~/.config/autostart desktop file enable/disable
│   ├── logging/logging.go (+ _test) # slog setup + attribute key constants
│   ├── cli/cli.go                   # --dump / --watch / --connect / --disconnect / --power / --autostart (no GTK)
│   └── ui/                          # cgo / GTK only; talks to bluetooth.BluetoothManager via DeviceID
│       ├── app.go                   # Run(): gtk.Application, actions, activate/shutdown
│       ├── window.go                # main Window struct, event application, page logic, ops
│       ├── header.go                # HeaderBar + settings MenuButton
│       ├── device_row.go            # deviceRow widget + incremental update
│       ├── settings.go              # settings popover (4 check buttons)
│       ├── states.go                # status pages (loading/empty/off/noadapter/unavailable)
│       ├── banner.go                # error banner with Retry / dismiss
│       ├── css.go                   # CSS string + loader
│       ├── theme.go                 # follow org.gnome.desktop.interface color-scheme
│       └── x11win/
│           ├── x11win.go            # //go:build !nox11 — cgo Xlib helper (keep-above, skip-taskbar, move, position)
│           └── x11win_stub.go       # //go:build nox11 — all functions return false
├── data/
│   ├── com.example.BluetoothWidget.desktop
│   └── icons/com.example.BluetoothWidget.svg
├── scripts/{install.sh,uninstall.sh,run.sh}   # chmod +x
└── tests/integration/
    ├── doc.go                       # `package integration` (no build tag; keeps ./... happy)
    └── bluez_integration_test.go    # //go:build integration
```

App ID constant: `const AppID = "com.example.BluetoothWidget"`. Put it in `internal/ui/app.go`. `autostart` also needs it, so define it once in `internal/autostart` as `autostart.AppID` and have ui reference it.

---

## 2. `internal/bluetooth` design

### 2.1 Types (events.go, device.go, adapter.go, battery.go)

```go
// DeviceID is opaque to the UI. Internally it is the D-Bus object path string.
type DeviceID string

type BatteryLevel struct {
    Label   string `json:"label"`   // "" for the main battery; else e.g. "left", "right", "case"
    Percent int    `json:"percent"` // 0..100 (clamped)
    Path    dbus.ObjectPath `json:"-"`
}

type BluetoothDevice struct {
    ID        DeviceID        `json:"id"`
    Path      dbus.ObjectPath `json:"path"`
    Adapter   dbus.ObjectPath `json:"adapter"`
    Address   string          `json:"address"`
    Name      string          `json:"name"`
    Alias     string          `json:"alias"`
    Icon      string          `json:"icon"`
    Paired    bool            `json:"paired"`
    Connected bool            `json:"connected"`
    Trusted   bool            `json:"trusted"`
    Battery   *int            `json:"battery"`   // primary battery, nil if none (spec field)
    Batteries []BatteryLevel  `json:"batteries"` // all batteries, sorted: "" first, then by Label
}
func (d *BluetoothDevice) DisplayName() string // TrimSpace(Alias) -> TrimSpace(Name) -> Address
func (d *BluetoothDevice) Clone() *BluetoothDevice // deep copy (Battery pointer + Batteries slice)
func (d *BluetoothDevice) Equal(o *BluetoothDevice) bool // compares all exported fields incl. batteries

type Adapter struct {
    Path         dbus.ObjectPath `json:"path"`
    Address      string `json:"address"`
    Name         string `json:"name"`
    Alias        string `json:"alias"`
    Powered      bool   `json:"powered"`
    Discoverable bool   `json:"discoverable"`
    Pairable     bool   `json:"pairable"`
}
func (a *Adapter) Clone() *Adapter

type State struct {
    ServiceAvailable bool               `json:"service_available"` // bluetoothd reachable
    Adapter          *Adapter           `json:"adapter"`           // nil = no adapter
    Devices          []*BluetoothDevice `json:"devices"`           // paired devices on the chosen adapter only
}

type EventKind int
const (
    EventReset EventKind = iota // full state replaced (startup, bluetoothd restart, adapter switch)
    EventDeviceAdded            // a paired device became visible
    EventDeviceChanged          // a visible device's relevant fields changed
    EventDeviceRemoved          // device removed or unpaired
    EventAdapterChanged         // adapter properties changed (Adapter may be nil)
)
func (k EventKind) String() string // "reset","device-added","device-changed","device-removed","adapter-changed"

type Event struct {
    Kind     EventKind
    DeviceID DeviceID          // for device events
    Device   *BluetoothDevice  // clone; nil for removed
    Adapter  *Adapter          // clone; for AdapterChanged/Reset (nil = none)
    State    *State            // only for EventReset
}
```

Parsing (device.go / adapter.go / battery.go). **Always use comma-ok type assertions and ignore mismatched types.**
```go
const (
    ifaceAdapter = "org.bluez.Adapter1"
    ifaceDevice  = "org.bluez.Device1"
    ifaceBattery = "org.bluez.Battery1"
    bluezService = "org.bluez"
)
func applyDeviceProps(d *BluetoothDevice, props map[string]dbus.Variant) // sets only keys present
// Keys: Address(string) Name(string) Alias(string) Icon(string) Paired/Connected/Trusted(bool) Adapter(dbus.ObjectPath)
func invalidateDeviceProps(d *BluetoothDevice, names []string) // Name/Alias/Icon -> ""; bools -> false
func applyAdapterProps(a *Adapter, props map[string]dbus.Variant) // Address Name Alias (string); Powered Discoverable Pairable (bool)
func parseBatteryPercent(props map[string]dbus.Variant) (int, bool)
// Percentage is D-Bus 'y' => Go byte (uint8). Accept uint8; also accept int32/uint32/int for robustness. Clamp 0..100.
func batteryLabel(devicePath, batteryPath dbus.ObjectPath) string // "" if equal, else last path element after devicePath+"/"
func primaryBattery(levels []BatteryLevel) *int // the Label=="" level if present, else the minimum Percent; nil if empty
```
Multiple batteries: any Battery1 object whose path equals the device path **or** starts with `devicePath + "/"` is attached to that device, keyed by its path. Today BlueZ exposes one Battery1 on the device path. The model and UI already handle `len(Batteries) > 1`, so "Left 84% · Right 80% · Case 92%" needs no rewrite later.

### 2.2 icons.go
```go
const FallbackIcon = "bluetooth-symbolic"
func IconCandidates(bluezIcon string) []string // ordered candidates, ALWAYS ends with FallbackIcon
```
Mapping table (keyed by the BlueZ `Icon` value):
- audio-headphones → audio-headphones-symbolic, audio-headset-symbolic
- audio-headset → audio-headset-symbolic, audio-headphones-symbolic
- audio-card → audio-speakers-symbolic, audio-card-symbolic
- input-keyboard → input-keyboard-symbolic
- input-mouse → input-mouse-symbolic
- input-gaming → input-gaming-symbolic
- input-tablet → input-tablet-symbolic
- phone → phone-symbolic
- computer → computer-symbolic
- camera-photo → camera-photo-symbolic
- camera-video → camera-video-symbolic, camera-web-symbolic
- video-display → video-display-symbolic
- printer → printer-symbolic
- scanner → scanner-symbolic
- multimedia-player → multimedia-player-symbolic
- modem → modem-symbolic
- network-wireless → network-wireless-symbolic
- unknown or empty → nothing, so the list is just the fallback

No brand icons.

### 2.3 errors.go
```go
type ErrorKind int
const (
    KindUnknown ErrorKind = iota
    KindFailed; KindNotReady; KindNotAvailable; KindInProgress
    KindAuthCanceled; KindAuthRejected; KindAuthFailed; KindAuthTimeout
    KindOutOfRange   // Failed + message contains "page-timeout" | "Host is down" | "le-connection-abort-by-local"? (only first two required)
    KindRfkill       // message contains "rfkill"
    KindTimeout      // context.DeadlineExceeded or org.freedesktop.DBus.Error.NoReply
    KindServiceUnavailable // org.freedesktop.DBus.Error.ServiceUnknown / NameHasNoOwner / ErrServiceUnavailable
    KindPermission   // org.freedesktop.DBus.Error.AccessDenied, org.bluez.Error.NotAuthorized, NotPermitted
    KindBusy         // ErrBusy (local in-flight guard)
    KindNoDevice     // org.bluez.Error.DoesNotExist, org.freedesktop.DBus.Error.UnknownObject, ErrUnknownDevice
)
var (
    ErrBusy               = errors.New("operation already in progress")
    ErrUnknownDevice      = errors.New("unknown device")
    ErrNoAdapter          = errors.New("no bluetooth adapter")
    ErrServiceUnavailable = errors.New("bluetooth service unavailable")
)
type OpError struct {
    Op          string // "connect" | "disconnect" | "set-powered"
    Device      string // display name ("" for adapter ops)
    DBusName    string // e.g. "org.bluez.Error.Failed"
    DBusMessage string // e.g. "br-connection-page-timeout"
    Kind        ErrorKind
    Err         error
}
func (e *OpError) Error() string  // technical: "connect CMF Buds Pro 2: org.bluez.Error.Failed: br-connection-page-timeout"
func (e *OpError) Unwrap() error
func (e *OpError) Short() string  // row status text, e.g. "Unable to connect", "Not in range", "Authentication failed", "Timed out"
func (e *OpError) Friendly() string // banner text, e.g. "Could not connect to CMF Buds Pro 2. Make sure it is turned on and nearby."

// ConvertError returns nil for benign results:
//   op=="connect"    && name=="org.bluez.Error.AlreadyConnected" -> nil
//   op=="disconnect" && name=="org.bluez.Error.NotConnected"     -> nil
func ConvertError(op, device string, err error) error
```
Extracting the D-Bus error: godbus returns **`dbus.Error` by value** for method-error replies. `NewError` returns a pointer, so handle both:
```go
var de dbus.Error
var dep *dbus.Error
switch {
case errors.As(err, &de):  name, msg = de.Name, firstString(de.Body)
case errors.As(err, &dep): name, msg = dep.Name, firstString(dep.Body)
}
```
`firstString(body []any)` returns `body[0].(string)` if possible.

Friendly texts:
| Kind | Short | Friendly |
|---|---|---|
| Failed / Unknown | "Unable to connect" | "Could not connect to X." |
| OutOfRange / NotAvailable | "Not in range" | "Could not connect to X. Make sure it is turned on and nearby." |
| NotReady | "Bluetooth not ready" | "Bluetooth is not ready yet. Try again in a moment." |
| InProgress | "Connecting…" | none; the UI must not show the banner |
| Auth* | "Authentication failed" | "Could not connect to X: authentication failed. Try pairing it again in Settings." |
| Timeout | "Timed out" | "Connecting to X timed out." |
| Rfkill | — | "Bluetooth is blocked (airplane mode or hardware switch)." |
| Permission | — | "Permission denied by the system Bluetooth service." |
| ServiceUnavailable | — | "The Bluetooth service (bluetoothd) is not running." |

For `disconnect`, use "Could not disconnect X." and "Unable to disconnect". For `set-powered`, use "Could not turn Bluetooth on." plus the kind-specific text.

### 2.4 bus.go — the narrow, mockable D-Bus layer
```go
type managedObjects = map[dbus.ObjectPath]map[string]map[string]dbus.Variant

type bus interface {
    GetManagedObjects(ctx context.Context) (managedObjects, error)
    Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) error // method = "org.bluez.Device1.Connect"
    SetProperty(ctx context.Context, path dbus.ObjectPath, iface, prop string, value any) error
    Signals() <-chan *dbus.Signal
    Close() error
}

type systemBus struct { conn *dbus.Conn; ch chan *dbus.Signal }
func newSystemBus() (*systemBus, error)
```
Steps inside `newSystemBus`:
1. Call `dbus.ConnectSystemBus()`. This gives a private connection. Do **not** use the shared `dbus.SystemBus()`.
2. Add these match rules with `conn.AddMatchSignal(...)`. Return the error if any rule fails.
   - Three PropertiesChanged rules, one for each iface in {Device1, Battery1, Adapter1}. Filtering on arg0 keeps out the GATT characteristic noise.
     `dbus.WithMatchSender("org.bluez"), dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchPathNamespace("/org/bluez"), dbus.WithMatchArg(0, iface)`
   - `WithMatchSender("org.bluez"), WithMatchInterface("org.freedesktop.DBus.ObjectManager"), WithMatchMember("InterfacesAdded")`
   - The same rule with member `"InterfacesRemoved"`.
   - `WithMatchSender("org.freedesktop.DBus"), WithMatchInterface("org.freedesktop.DBus"), WithMatchMember("NameOwnerChanged"), WithMatchArg(0, "org.bluez")`
3. Create `ch := make(chan *dbus.Signal, 64)` and call `conn.Signal(ch)`. godbus closes `ch` when the connection closes.

Method implementations:
- `GetManagedObjects`: `conn.Object("org.bluez", "/").CallWithContext(ctx, "org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&out)`
- `Call`: `conn.Object("org.bluez", path).CallWithContext(ctx, method, 0, args...).Err`
- `SetProperty`: `conn.Object("org.bluez", path).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, iface, prop, dbus.MakeVariant(value)).Err`
- `Close`: `conn.RemoveSignal(ch)` then `conn.Close()`.

### 2.5 store.go — pure state machine (the core of the unit tests)
```go
type store struct {
    serviceUp  bool
    adapters   map[dbus.ObjectPath]*Adapter
    devices    map[dbus.ObjectPath]*BluetoothDevice   // ALL devices (paired or not), batteries attached
    batteries  map[dbus.ObjectPath]batteryRec         // battery path -> {percent}
    current    dbus.ObjectPath                        // chosen adapter ("" = none)
}
func newStore() *store
func (s *store) reset(objs managedObjects)                 // rebuild from scratch; serviceUp=true; choose adapter
func (s *store) setServiceDown()                           // clear all; serviceUp=false
func (s *store) snapshot() State                            // clones; devices = visible() sorted by path (UI re-sorts)
func (s *store) visible(d *BluetoothDevice) bool            // d.Paired && d.Adapter == s.current && s.current != ""
func (s *store) interfacesAdded(path dbus.ObjectPath, ifaces map[string]map[string]dbus.Variant) []Event
func (s *store) interfacesRemoved(path dbus.ObjectPath, ifaces []string) []Event
func (s *store) propertiesChanged(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant, invalidated []string) []Event
func (s *store) device(id DeviceID) (*BluetoothDevice, bool)  // clone, visible only
func pickAdapter(adapters map[dbus.ObjectPath]*Adapter, current dbus.ObjectPath) dbus.ObjectPath
// keep current if it still exists; else lexicographically smallest path (hci0 before hci1); "" if none
```
Change-event rule. Use one helper for every device mutation:
```go
func (s *store) deviceEvents(path, before *BluetoothDevice /*clone or nil*/, after *BluetoothDevice) []Event
```
- `wasVisible && !isVisible` → `DeviceRemoved`
- `!wasVisible && isVisible` → `DeviceAdded`
- both visible and `!before.Equal(after)` → `DeviceChanged`
- otherwise nothing. This means RSSI, ManufacturerData or UUIDs-only changes emit **no** event.

Adapter rules:
- Adapter added or removed, and `pickAdapter` result changes → emit a single `EventReset` with a full snapshot.
- Current adapter property change → `EventAdapterChanged` only if the clone differs.
- Battery1 added, changed or removed → recompute `Batteries`/`Battery` of the owning device, then run `deviceEvents`.
- `InterfacesAdded` for an already-known path → merge the props. Never replace the device, because Battery1 may arrive separately from Device1.
- `InterfacesRemoved` containing Device1 → delete the device and any batteries under it.

### 2.6 manager.go
```go
type BluetoothManager interface {
    Start(ctx context.Context) error                 // connect, subscribe, initial load, emits EventReset, starts loop
    Snapshot() State
    ListPairedDevices() ([]*BluetoothDevice, error)  // from cache; ErrServiceUnavailable if down
    ConnectDevice(ctx context.Context, id DeviceID) error
    DisconnectDevice(ctx context.Context, id DeviceID) error
    SetAdapterPowered(ctx context.Context, powered bool) error
    SubscribeToChanges(fn func(Event)) (unsubscribe func())
    Close() error
}

func NewSystemManager(logger *slog.Logger) BluetoothManager            // uses newSystemBus lazily in Start
func newManager(b func() (bus, error), logger *slog.Logger) *systemManager // for tests (inject fake)

type systemManager struct {
    newBus  func() (bus, error)
    bus     bus
    log     *slog.Logger
    mu      sync.RWMutex   // guards st
    st      *store
    subMu   sync.Mutex
    subs    map[int]func(Event); nextSub int
    opsMu   sync.Mutex
    inflight map[DeviceID]string // id -> op
    powerBusy bool
    ctx     context.Context; cancel context.CancelFunc
    done    chan struct{}
}
```
Timeout constants: `connectTimeout = 40*time.Second`, `disconnectTimeout = 20*time.Second`, `powerTimeout = 10*time.Second`, `loadTimeout = 10*time.Second`.

**`Start` sequence** (it is called from a goroutine, never from the GTK thread):
1. Call `b, err := m.newBus()`. If it fails, log it, set the store to service down, emit a Reset (ServiceAvailable=false), and return a wrapped `ErrServiceUnavailable`.
2. Call `objs, err := b.GetManagedObjects(ctx with loadTimeout)`.
   - If err is ServiceUnknown/NameHasNoOwner: set service down, emit Reset, **still start the loop** so that NameOwnerChanged brings the app back, and return nil.
   - Other errors: log them and treat them the same way.
3. Under `mu.Lock`, call `st.reset(objs)` and take a snapshot. Then unlock and call `emit(Event{Kind: EventReset, State: &snap, Adapter: snap.Adapter})`.
4. Run `go m.loop()`. This is the **only** long-lived goroutine.

**`loop`**:
- `for { select { case <-m.ctx.Done(): return; case sig, ok := <-m.bus.Signals(): if !ok { return }; m.handleSignal(sig) } }`
- Close `m.done` on exit.

**`handleSignal`** switches on `sig.Name`:
- `"org.freedesktop.DBus.Properties.PropertiesChanged"`: `iface, _ := sig.Body[0].(string); changed, _ := sig.Body[1].(map[string]dbus.Variant); inval, _ := sig.Body[2].([]string)`. Guard with `len(sig.Body) >= 3`. Ignore interfaces other than the three bluez ones.
- `"org.freedesktop.DBus.ObjectManager.InterfacesAdded"`: `path := sig.Body[0].(dbus.ObjectPath); ifaces := sig.Body[1].(map[string]map[string]dbus.Variant)` (use comma-ok).
- `"org.freedesktop.DBus.ObjectManager.InterfacesRemoved"`: `path, []string`.
- `"org.freedesktop.DBus.NameOwnerChanged"`: `name, old, new string`. Act only when `name == "org.bluez"`.
  - `new == ""` → `setServiceDown` and Reset.
  - otherwise → `GetManagedObjects` (ctx 10s), `reset`, and Reset. A few InterfacesAdded signals will also arrive; the merge logic makes them idempotent.
- Every other signal name, including `NameAcquired`, is ignored.
- Pattern: take `mu.Lock`, compute `events := st.xxx(...)`, `mu.Unlock()`, then `for _, e := range events { m.emit(e) }`. **Never call subscribers while holding `mu`.**

**`emit`** copies the subscriber funcs under `subMu` and calls each one synchronously. Subscribers must not block; the UI's subscriber only calls `glib.IdleAdd`.

**`ConnectDevice` / `DisconnectDevice`**:
1. Take `mu.RLock`, look up the device clone, and release the lock. If it is missing, return `ErrUnknownDevice` (via ConvertError, Kind NoDevice).
2. `if !m.beginOp(id, op) { return &OpError{Op: op, Kind: KindBusy, Err: ErrBusy, Device: name} }`, then `defer m.endOp(id)`.
3. `ctx, cancel := context.WithTimeout(ctx, connectTimeout)`.
4. `slog.Info("connect", "op","connect","device",name,"address",addr,"path",path)`.
5. `err := m.bus.Call(ctx, path, "org.bluez.Device1.Connect")` (or `...Disconnect`).
6. `cerr := ConvertError(op, name, err)`. If non-nil, log at ERROR with `"error", cerr.Error(), "dbus_error", oe.DBusName, "dbus_message", oe.DBusMessage`. Otherwise log INFO `"result","ok"`.
7. Return `cerr`.
Note: cancelling the context does **not** cancel BlueZ's operation. The UI still receives the final state through PropertiesChanged.

**`SetAdapterPowered`**:
- Take the current adapter path under RLock. Return `ErrNoAdapter` if there is none.
- Guard with `powerBusy`.
- Call `m.bus.SetProperty(ctx(10s), path, "org.bluez.Adapter1", "Powered", powered)`, then `ConvertError("set-powered", "", err)`.

**`Close`**: cancel ctx, call `bus.Close()`, and wait for `done` with a 2s timeout. It must be idempotent (use `sync.Once`).

---

## 3. Other pure packages

### 3.1 `internal/order`
```go
type Group int
const (GroupConnected Group = iota; GroupFavorite; GroupOther)
type Sorter interface {
    Group(d *bluetooth.BluetoothDevice) Group
    Compare(a, b *bluetooth.BluetoothDevice) int // <0, 0, >0
}
type DefaultSorter struct{ IsFavorite func(address string) bool }
```
Order: group ascending, then `strings.ToLower(DisplayName())`, then `Address`. A connected favorite counts as GroupConnected.

### 3.2 `internal/config`
```go
type WindowState struct {
    Width  int  `json:"width"`
    Height int  `json:"height"`
    X      *int `json:"x,omitempty"` // only saved/used on X11
    Y      *int `json:"y,omitempty"`
}
type Config struct {
    Version          int         `json:"version"`            // 1
    AlwaysOnTop      bool        `json:"always_on_top"`      // default false
    ShowBattery      bool        `json:"show_battery"`       // default true
    StartAtLogin     bool        `json:"start_at_login"`     // default false (autostart file is source of truth)
    ShowDisconnected bool        `json:"show_disconnected"`  // default true
    Favorites        []string    `json:"favorites"`          // upper-case MAC "AA:BB:CC:DD:EE:FF", sorted, deduped
    Window           WindowState `json:"window"`             // default 340x440
}
func Default() Config
func DefaultPath() (string, error) // $XDG_CONFIG_HOME/bluetooth-widget/config.json; else os.UserHomeDir()+"/.config/..."
func NormalizeMAC(s string) string // strings.ToUpper(strings.TrimSpace(s))
type Store struct { mu sync.Mutex; path string; cfg Config }
func Load(path string) (*Store, error) // ALWAYS returns a usable *Store (defaults); err non-nil only if file existed but was unreadable/corrupt
func (s *Store) Path() string
func (s *Store) Get() Config                         // deep copy
func (s *Store) Update(fn func(c *Config)) error     // lock, fn, normalize, save atomically
func (s *Store) IsFavorite(addr string) bool
func (s *Store) SetFavorite(addr string, fav bool) error
func Save(path string, c Config) error
```
- Load defaults first and then `json.Unmarshal` on top. This way missing fields keep their defaults. For example, a file without `show_battery` gets `true`.
- If the file is corrupt, log a warning, keep the defaults, and do **not** overwrite the file until the next `Update`.
- **Atomic save**:
  1. `os.MkdirAll(dir, 0o700)`
  2. `f, _ := os.CreateTemp(dir, ".config-*.json")`
  3. Write `json.MarshalIndent(c, "", "  ")` plus a trailing newline.
  4. `f.Sync()`, `f.Close()`, `os.Chmod(tmp, 0o600)`, `os.Rename(tmp, path)`.
  5. On any error, `os.Remove(tmp)`.
- Only preferences and MACs are stored. Never store link keys or pairing data.

### 3.3 `internal/autostart`
```go
const AppID = "com.example.BluetoothWidget"
func Dir() (string, error)          // $XDG_CONFIG_HOME/autostart or ~/.config/autostart
func FilePath() (string, error)     // Dir()/com.example.BluetoothWidget.desktop
func IsEnabled() bool               // file exists and does not contain "Hidden=true" / "X-GNOME-Autostart-enabled=false"
func Enable(execPath string) error  // atomic write (same temp+rename pattern), 0644
func Disable() error                // os.Remove; ignore fs.ErrNotExist
func Content(execPath string) string
func QuoteExec(p string) string     // Desktop Entry spec: if p contains space, tab, newline, ", ', \, >, <, ~, |, &, ;, $, *, ?, #, (, ), `
                                    // wrap in "..." and escape ", `, $, \ with backslash
func ResolveExecutable() (string, error) // os.Executable() + filepath.EvalSymlinks; if path contains "/go-build" return error "run the installed binary to enable autostart"
```
Content:
```
[Desktop Entry]
Type=Application
Name=Bluetooth Widget
Comment=Connect and disconnect paired Bluetooth devices with one click
Exec=<QuoteExec(path)>
Icon=com.example.BluetoothWidget
Terminal=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=3
```

### 3.4 `internal/logging`
```go
const (KeyOp="op"; KeyDevice="device"; KeyAddress="address"; KeyPath="path"; KeyError="error"; KeyDBusError="dbus_error"; KeyDBusMessage="dbus_message")
func LevelFromEnv(debug bool) slog.Level // debug flag wins; else env BLUETOOTH_WIDGET_LOG in {debug,info,warn,error}; default Info
func Setup(w io.Writer, level slog.Level) *slog.Logger // slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}); slog.SetDefault; return
```
Log to stderr. The text handler includes `time=` automatically.

### 3.5 `internal/cli` (no GTK)
```go
func Dump(ctx context.Context, m bluetooth.BluetoothManager, w io.Writer) int    // Start, json.MarshalIndent(m.Snapshot()), Close; exit 0/1
func Watch(ctx context.Context, m bluetooth.BluetoothManager, w io.Writer) int   // Subscribe (prints JSON line per event: {"time","kind","device_id","device","adapter","state"}), Start, block until ctx done (signal.NotifyContext SIGINT/SIGTERM)
func ConnectByAddress(ctx, m, addr string, connect bool, w io.Writer) int        // Start, find device by NormalizeMAC(address), call op, print friendly result
func SetPower(ctx, m, on bool, w io.Writer) int
func SetAutostart(on bool, cfg *config.Store, w io.Writer) int                   // Enable(ResolveExecutable())/Disable + cfg.Update(StartAtLogin)
```

### 3.6 `cmd/bluetooth-widget/main.go`
```go
var version = "dev" // -ldflags "-X main.version=..."
func init() { runtime.LockOSThread() } // GTK must stay on the main OS thread (added in Phase 3; harmless before)
func main() { os.Exit(run()) }
```
Flags (standard `flag` package):
- `-dump`
- `-watch`
- `-connect MAC`
- `-disconnect MAC`
- `-power on|off`
- `-autostart on|off`
- `-debug`
- `-version`

Run order:
1. Parse flags.
2. `logging.Setup(os.Stderr, LevelFromEnv(debug))`.
3. `config.Load(DefaultPath())`.
4. If a CLI flag is set, dispatch to `cli.*` and **do not touch GTK**.
5. Otherwise `return ui.Run(ui.Options{...})`.

In Phase 1 and Phase 2 there is no UI yet. Running with no flags prints usage to stderr and returns 2.

---

## 4. GTK UI design (`internal/ui`, GTK 4.6 via gotk4 v0.2.2)

### 4.1 Threading contract (must follow exactly)
- Every GTK call happens on the main thread: inside `activate`, signal handlers, or `glib.IdleAdd`/`glib.TimeoutAdd` callbacks.
- Manager to UI: in `activate`, register the subscriber **before** starting the manager:
  ```go
  unsub := opts.Manager.SubscribeToChanges(func(ev bluetooth.Event) {
      glib.IdleAdd(func() { w.handleEvent(ev) }) // func() with no return = runs once
  })
  go func() {
      if err := opts.Manager.Start(ctx); err != nil { slog.Warn(...) } // Start emits EventReset itself
  }()
  ```
  GLib dispatches same-priority idle sources in FIFO order, so events keep their order.
- Operations: one short-lived goroutine per user action. Its result is marshalled back with `glib.IdleAdd`:
  ```go
  func (w *Window) startOp(id bluetooth.DeviceID, op opKind) {
      r := w.rows[id]; if r == nil || r.busy != opNone { return }   // UI-level guard
      r.setBusy(op)
      go func() {
          var err error
          if op == opConnect { err = w.mgr.ConnectDevice(w.ctx, id) } else { err = w.mgr.DisconnectDevice(w.ctx, id) }
          glib.IdleAdd(func() { w.finishOp(id, op, err) })
      }()
  }
  ```
- `glib.IdleAdd` in v0.2.2 has the signature `IdleAdd(f interface{}) SourceHandle`. `f` must be `func()` or `func() bool`; if it returns `true` it is re-run. Anything else panics.
- **Never** capture and mutate a `*BluetoothDevice` from the manager across threads. Events carry clones, and the UI stores its own copy.

### 4.2 `app.go`
```go
type Options struct {
    Manager bluetooth.BluetoothManager
    Config  *config.Store
    Logger  *slog.Logger
    Version string
}
func Run(opts Options) int
```
- `app := gtk.NewApplication(autostart.AppID, gio.ApplicationFlagsNone)`.
- Single instance comes for free: a second launch activates the first instance and exits 0.
- `app.ConnectStartup(func(){ loadCSS(); setupTheme() })`.
- `app.ConnectActivate(func(){ if w != nil { w.win.Present(); return }; w = newWindow(app, opts); w.win.Present() })`.
- Add actions:
  - `quit := gio.NewSimpleAction("quit", nil); quit.ConnectActivate(func(_ *glib.Variant){ w.saveState(); app.Quit() }); app.AddAction(quit)`
  - `app.SetAccelsForAction("app.quit", []string{"<Control>q"})`
  - `app.SetAccelsForAction("window.close", []string{"<Control>w"})`
- `app.ConnectShutdown(func(){ if w != nil { w.saveState() }; opts.Manager.Close() })`.
- `return app.Run([]string{os.Args[0]})`. **Only argv[0]**, otherwise GApplication rejects our Go flags.

### 4.3 `window.go`
```go
type Window struct {
    app     *gtk.Application
    win     *gtk.ApplicationWindow
    mgr     bluetooth.BluetoothManager
    cfg     *config.Store
    sorter  order.Sorter
    ctx     context.Context; cancel context.CancelFunc

    stack   *gtk.Stack            // pages: "loading","devices","empty","off","noadapter","unavailable"
    list    *gtk.ListBox
    rows    map[bluetooth.DeviceID]*deviceRow
    banner  *errorBanner
    header  *header
    settings *settingsPopover
    pages   *statusPages

    serviceUp bool
    adapter   *bluetooth.Adapter
    loaded    bool                // first Reset received
    powerBusy bool
    x11       bool                // x11win.Supported()
}
func newWindow(app *gtk.Application, o Options) *Window
func (w *Window) handleEvent(ev bluetooth.Event)
func (w *Window) applyReset(st *bluetooth.State)
func (w *Window) upsertDevice(d *bluetooth.BluetoothDevice)
func (w *Window) removeDevice(id bluetooth.DeviceID)
func (w *Window) updatePage()
func (w *Window) startOp(id bluetooth.DeviceID, op opKind)
func (w *Window) finishOp(id bluetooth.DeviceID, op opKind, err error)
func (w *Window) powerOn()
func (w *Window) toggleFavorite(id bluetooth.DeviceID)
func (w *Window) applyWindowHints()   // X11 only
func (w *Window) saveState()
```
Construction:
- `win := gtk.NewApplicationWindow(app)`, `SetTitle("Bluetooth")`, `SetIconName(autostart.AppID)`.
- `SetDefaultSize(cfg.Window.Width, cfg.Window.Height)`. Clamp to at least 280x200; the default is 340x440.
- `SetSizeRequest(280, 200)`.
- `SetTitlebar(header.bar)`.
- Content is a vertical `gtk.NewBox(gtk.OrientationVertical, 0)` containing the banner revealer and then the stack.
- `"devices"` page: `sw := gtk.NewScrolledWindow(); sw.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic); sw.SetVExpand(true); sw.SetChild(list)`.
- List setup:
  - `list := gtk.NewListBox(); list.SetSelectionMode(gtk.SelectionNone); list.SetActivateOnSingleClick(true); list.AddCSSClass("navigation-sidebar"); list.AddCSSClass("bw-list")`.
  - `list.SetSortFunc(func(a, b *gtk.ListBoxRow) int { ra, rb := w.rows[DeviceID(a.Name())], w.rows[DeviceID(b.Name())]; if ra==nil||rb==nil {return 0}; return w.sorter.Compare(ra.dev, rb.dev) })`
    **gotk4 passes fresh wrapper objects to these callbacks. Identify rows with `row.Name()`, which you set with `row.SetName(string(id))`. Never compare pointers.**
  - `list.SetFilterFunc(func(r *gtk.ListBoxRow) bool { dr := w.rows[...]; return dr != nil && (w.cfg.Get().ShowDisconnected || dr.dev.Connected || dr.busy != opNone) })`
  - `list.SetHeaderFunc(func(row, before *gtk.ListBoxRow) { ... })`. Compute the section with `sectionOf(dev)`: "CONNECTED" if Connected, else "PAIRED". If `before == nil` or the section differs from before's, call `row.SetHeader(newSectionLabel(text))`; otherwise call `row.SetHeader(nil)`. Pass a **literal `nil`**, never a typed nil pointer.
  - `list.ConnectRowActivated(func(r *gtk.ListBoxRow){ dr := w.rows[DeviceID(r.Name())]; if dr==nil||dr.busy!=opNone {return}; if dr.dev.Connected { w.startOp(dr.id, opDisconnect) } else { w.startOp(dr.id, opConnect) } })`
- `order.DefaultSorter{IsFavorite: w.cfg.IsFavorite}`.
- Window lifecycle:
  - `win.ConnectCloseRequest(func() bool { w.saveState(); return false })`. Returning false allows the close, and the app exits when its last window closes.
  - `win.ConnectMap(func(){ glib.TimeoutAdd(150, func(){ w.applyWindowHints() }) })`. The timeout is one-shot because the func returns nothing.
- Initial page is `"loading"`.

`handleEvent`:
- **Reset** → `applyReset`: remove every row with `list.Remove(r.row)`, clear the map, set `serviceUp` and `adapter`, add a row per device, then `updatePage()`, `settings.refresh()`, and set `loaded = true`. A full rebuild is only allowed here.
- **DeviceAdded / DeviceChanged** → `upsertDevice(ev.Device)`.
  - If the row exists, `changed := r.update(d, opts)`. If the sort key, section or filter result changed (connected state or name), call `r.row.Changed()` and `list.InvalidateHeaders()`.
  - Otherwise create the row, `list.Append(r.row)`, and store it in the map.
  - If `d.Connected` and the banner refers to this id, hide the banner.
- **DeviceRemoved** → `list.Remove(r.row)`, delete from the map, hide the banner if it refers to this id.
- **AdapterChanged** → `w.adapter = ev.Adapter`.

After every event, call `updatePage()`.

`updatePage` (decision order):
```
!loaded                          -> "loading"
!serviceUp                       -> "unavailable"
adapter == nil                   -> "noadapter"
!adapter.Powered                 -> "off"
visibleCount()==0 && len(rows)==0-> "empty" (title "No paired devices", subtitle "Pair a Bluetooth device to see it here.")
visibleCount()==0                -> "empty" (title "No connected devices", subtitle "Turn on “Show disconnected devices” in settings to see all paired devices.")
else                             -> "devices"
```
`visibleCount` applies the same predicate as the filter func. Call `stack.SetVisibleChildName(name)` only if the name differs.

`finishOp(id, op, err)`:
- `r.clearBusy()`, then refresh from `r.dev`.
- `err == nil` → nothing more. The row already reflects PropertiesChanged.
- `errors.As(err, &oe)`:
  - Kind InProgress or Busy → ignore.
  - Otherwise `r.showError(oe.Short())` (the error style stays until the next update or op) and `w.banner.show(oe.Friendly(), func(){ w.startOp(id, op) }, id)`.
- Non-OpError errors → generic "Unable to connect".

`powerOn`:
- Set the off-page button insensitive and show its spinner.
- `go` → `mgr.SetAdapterPowered(ctx, true)` → IdleAdd → re-enable the button. On error, set the page's error label to `oe.Friendly()`.
- The page switches automatically when the AdapterChanged(Powered=true) event arrives.

`saveState` (main thread):
- `if !win.IsMaximized() { wdt, hgt := win.DefaultSize(); if wdt <= 0 { wdt = win.Width() }; if hgt <= 0 { hgt = win.Height() } }`.
- If `w.x11`, `x, y, ok := x11win.Position(&win.Window)`.
- `cfg.Update(func(c){ c.Window = ... })`. Log errors at WARN.

### 4.4 `device_row.go`
```go
type opKind int
const (opNone opKind = iota; opConnect; opDisconnect)

type rowCallbacks struct{ onFavorite func(id bluetooth.DeviceID) }

type deviceRow struct {
    id       bluetooth.DeviceID
    dev      *bluetooth.BluetoothDevice
    favorite bool
    busy     opKind
    errText  string

    row      *gtk.ListBoxRow
    icon     *gtk.Image
    name     *gtk.Label
    status   *gtk.Label
    battery  *gtk.Label
    battIcon *gtk.Image
    spinner  *gtk.Spinner
    star     *gtk.Button
}
func newDeviceRow(d *bluetooth.BluetoothDevice, favorite bool, cb rowCallbacks, showBattery bool) *deviceRow
func (r *deviceRow) update(d *bluetooth.BluetoothDevice, favorite, showBattery bool) (sortKeyChanged bool)
func (r *deviceRow) setBusy(op opKind)   // spinner.Show+Start, battery hidden, status "Connecting…"/"Disconnecting…", tooltip
func (r *deviceRow) clearBusy()          // spinner.Stop+Hide (SetVisible(false)), re-render from r.dev
func (r *deviceRow) showError(short string) // status text short, css class "error"
func (r *deviceRow) render(showBattery bool) // single place that sets all texts/classes/tooltips/a11y from state
```
Layout:
- `row := gtk.NewListBoxRow(); row.SetName(string(id)); row.SetActivatable(true); row.AddCSSClass("bw-row")`
- `hbox := gtk.NewBox(gtk.OrientationHorizontal, 10)`
- `icon := gtk.NewImage(); icon.SetPixelSize(24)`. Set the icon name to the first of `bluetooth.IconCandidates(d.Icon)` for which `gtk.IconThemeGetForDisplay(gdk.DisplayGetDefault()).HasIcon(name)`.
- `vbox := gtk.NewBox(gtk.OrientationVertical, 2); vbox.SetHExpand(true)`
- `name := gtk.NewLabel(""); name.SetXAlign(0); name.SetEllipsize(pango.EllipsizeEnd); name.AddCSSClass("bw-name")`
- `line2 := gtk.NewBox(gtk.OrientationHorizontal, 6)`
  - `status` label: xalign 0, hexpand, class `bw-status`.
  - `battIcon` image with pixel size 16.
  - `battery` label with class `bw-battery`.
  - `spinner := gtk.NewSpinner()`, hidden by default.
- `star := gtk.NewButtonFromIconName("non-starred-symbolic"); star.SetHasFrame(false); star.SetVAlign(gtk.AlignCenter); star.AddCSSClass("flat"); star.ConnectClicked(func(){ cb.onFavorite(id) })`. The button takes the click, so clicking the star does not activate the row.
- Append `icon, vbox(name, line2), star` to `hbox`, then `row.SetChild(hbox)`.

`render` rules:

| State | status text | css on status | right side |
|---|---|---|---|
| busy connect | "Connecting…" | — | spinner (battery hidden) |
| busy disconnect | "Disconnecting…" | — | spinner |
| errText != "" | errText | `error` | battery if any |
| Connected | "● Connected" | `connected` | battery |
| otherwise | "○ Click to connect" | `dim-label` | battery (BlueZ usually drops Battery1 on disconnect) |

Battery rules (only when `showBattery && len(d.Batteries) > 0`):
- One battery: `"82%"`. Icon `battery-level-%d-symbolic` with `%d = (p/10)*10`, set only if HasIcon.
- Several batteries: `"L 84% · R 80% · Case 92%"`. Label = first letter upper-cased for left/right, otherwise the capitalised label. No icon.
- Otherwise hide both.

Star: `starred-symbolic` with tooltip "Remove from favorites", or `non-starred-symbolic` with "Add to favorites". Set the same text as the accessible label.

Tooltips:
- Row: `"<Name> (<Address>) — click to connect"` / `"… — click to disconnect"` / `"… — working…"`.
- Battery: `"Battery: 82%"`.

Accessible label (screen readers):
```go
func setA11yLabel(w *gtk.Widget, text string) {
    v := coreglib.NewValue(text)
    w.UpdateProperty([]gtk.AccessibleProperty{gtk.AccessiblePropertyLabel}, []coreglib.Value{*v})
}
```
Call it on the row with `"<Name>, Connected, battery 82%"` and on the star button. Reach the embedded Widget with `&r.row.Widget`, or use `gtk.BaseWidget(r.row)`. If this call gives trouble on 4.6, drop it and keep the tooltips. It is non-critical.

While `busy != opNone`, still apply name, icon and battery updates from events, but keep the busy status text.

Sort-key change: return true from `update` if any of these differ from before: `Connected`, `favorite`, `DisplayName()`.

### 4.5 `header.go`
```go
type header struct{ bar *gtk.HeaderBar; menu *gtk.MenuButton }
func newHeader(settings *gtk.Popover) *header
```
- `bar := gtk.NewHeaderBar()`.
- `title := gtk.NewLabel("Bluetooth"); title.AddCSSClass("title"); bar.SetTitleWidget(title)`.
- `menu := gtk.NewMenuButton(); menu.SetIconName("open-menu-symbolic"); menu.SetTooltipText("Settings"); menu.SetPopover(settings); bar.PackEnd(menu)`.
- Add `setA11yLabel` "Settings" on the menu button.

### 4.6 `settings.go`
```go
type settingsPopover struct {
    pop *gtk.Popover
    alwaysOnTop, showBattery, startAtLogin, showDisconnected *gtk.CheckButton
    adapterLabel *gtk.Label
    updating bool
}
func newSettingsPopover(w *Window) *settingsPopover
func (s *settingsPopover) refresh() // sets Active from cfg + autostart.IsEnabled(); adapter label; sets updating=true around SetActive calls
```
Contents: a vertical box with margin 12 and spacing 6:
- `gtk.NewCheckButtonWithLabel("Always on top")`
  - On X11: toggling calls `cfg.Update(AlwaysOnTop)`, then `x11win.SetKeepAbove(win, v)` and `x11win.SetSkipTaskbar(win, v)`.
  - On non-X11: `SetSensitive(false)` and tooltip: *"Not available on Wayland. Use your desktop's window menu (Alt+Space → Always on Top) instead."*
- `"Show battery level"` → `cfg.Update`, then re-render all rows.
- `"Start at login"` → `autostart.Enable(ResolveExecutable())` / `Disable()`, then `cfg.Update(StartAtLogin)`. On error, revert the check (inside the `updating` guard) and set the tooltip to the error text.
- `"Show disconnected devices"` → `cfg.Update`, `list.InvalidateFilter()`, `list.InvalidateHeaders()`, `updatePage()`.
- `gtk.NewSeparator(gtk.OrientationHorizontal)`.
- Dim adapter info label: `"Adapter: <Alias> (<Address>)"`, or `"No adapter"`.

Every `ConnectToggled` handler must start with `if s.updating { return }`, because programmatic `SetActive` fires `toggled` too.

### 4.7 `states.go`
`newStatusPage(iconName, title, subtitle string) *statusPage`. It is a vertical box, centered with valign/halign center and spacing 8:
- Image (pixel size 48, class `dim-label`)
- Title label (class `bw-empty-title`)
- Subtitle label (`dim-label`, wrap, max-width-chars 30, justify center)
- Optional action button (class `suggested-action`) with a spinner
- Optional error label (class `error`, hidden)

Pages:

| name | icon | title | subtitle / action |
|---|---|---|---|
| loading | — (Spinner started) | "Loading…" | — |
| empty | bluetooth-symbolic | (per updatePage) | (per updatePage) |
| off | bluetooth-disabled-symbolic | "Bluetooth is turned off." | Button "Turn Bluetooth On" → `w.powerOn()` |
| noadapter | bluetooth-disabled-symbolic | "No Bluetooth adapter detected." | "Connect a Bluetooth adapter or check that it is enabled." |
| unavailable | dialog-error-symbolic | "Bluetooth service unavailable" | "The BlueZ service (bluetoothd) is not running. The widget will reconnect automatically when it starts." |

Stop the loading spinner once the page changes away from loading. Nothing should animate while idle.

### 4.8 `banner.go`
```go
type errorBanner struct {
    rev *gtk.Revealer
    label *gtk.Label
    retry, closeBtn *gtk.Button
    deviceID bluetooth.DeviceID
    onRetry func()
}
func newErrorBanner() *errorBanner
func (b *errorBanner) show(text string, retry func(), id bluetooth.DeviceID)
func (b *errorBanner) hide()
func (b *errorBanner) refersTo(id bluetooth.DeviceID) bool
```
- `gtk.NewRevealer()` with `SetTransitionType(gtk.RevealerTransitionTypeSlideDown)` containing an hbox of class `bw-banner`:
  - `dialog-error-symbolic` icon
  - Label (wrap, hexpand, xalign 0)
  - `gtk.NewButtonWithLabel("Retry")`
  - `gtk.NewButtonFromIconName("window-close-symbolic")` with class `flat` and tooltip "Dismiss"
- Retry hides the banner and calls `onRetry`.

### 4.9 `css.go`
```go
const appCSS = `
.bw-list { background: transparent; }
.bw-row { padding: 6px 8px; border-radius: 8px; }
.bw-name { font-weight: 600; }
.bw-status, .bw-battery { font-size: 0.9em; }
.bw-status.connected { color: #26a269; }
.bw-status.error, label.error { color: #e01b24; }
.bw-battery { opacity: 0.8; }
.bw-section { font-size: 0.75em; font-weight: 700; opacity: 0.6; margin: 8px 12px 2px 12px; }
.bw-empty-title { font-size: 1.15em; font-weight: 700; }
.bw-banner { background-color: alpha(#e01b24, 0.15); border-radius: 8px; padding: 6px 8px; margin: 6px; }
`
func loadCSS() {
    p := gtk.NewCSSProvider()
    p.LoadFromData(appCSS)                              // v0.2.2 signature: LoadFromData(string)
    gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), p, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
}
```
Hover and pressed styles come from `navigation-sidebar` and the theme. Use only `alpha()`/currentColor-relative or neutral colours, so it works in dark and light. Do not use `@accent_color` or other libadwaita-only names.

### 4.10 `theme.go` — follow the system dark/light preference
```go
func setupTheme() {
    src := gio.SettingsSchemaSourceGetDefault()
    if src == nil { return }
    schema := src.Lookup("org.gnome.desktop.interface", true)
    if schema == nil || !schema.HasKey("color-scheme") { return } // g_settings_new ABORTS on missing schema — always check first
    s := gio.NewSettings("org.gnome.desktop.interface")
    apply := func() { gtk.SettingsGetDefault().SetObjectProperty("gtk-application-prefer-dark-theme", s.String("color-scheme") == "prefer-dark") }
    apply()
    s.ConnectChanged(func(key string) { if key == "color-scheme" { apply() } })
    keepSettings = s // package-level var so it is not garbage collected
}
```

### 4.11 `x11win` — always-on-top, taskbar and position (X11 only)
`x11win.go` (build tag `//go:build !nox11`):
```go
package x11win

/*
#cgo pkg-config: gtk4 x11
#include <stdint.h>
#include <string.h>
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>
#include <X11/Xlib.h>
#include <X11/Xatom.h>

static int bw_is_x11(void) {
    GdkDisplay *d = gdk_display_get_default();
    return d != NULL && GDK_IS_X11_DISPLAY(d);
}
static GdkSurface *bw_surface(uintptr_t w) {
    GtkNative *n = gtk_widget_get_native(GTK_WIDGET((gpointer)w));
    if (n == NULL) return NULL;
    GdkSurface *s = gtk_native_get_surface(n);
    if (s == NULL || !GDK_IS_X11_SURFACE(s)) return NULL;
    return s;
}
static int bw_wm_state(uintptr_t w, int add, const char *atom_name) {
    GdkSurface *s = bw_surface(w); if (!s) return 0;
    Display *dpy = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
    Window xid = gdk_x11_surface_get_xid(s);
    XEvent ev; memset(&ev, 0, sizeof ev);
    ev.xclient.type = ClientMessage;
    ev.xclient.window = xid;
    ev.xclient.message_type = XInternAtom(dpy, "_NET_WM_STATE", False);
    ev.xclient.format = 32;
    ev.xclient.data.l[0] = add ? 1 : 0;           // _NET_WM_STATE_ADD / _REMOVE
    ev.xclient.data.l[1] = XInternAtom(dpy, atom_name, False);
    ev.xclient.data.l[2] = 0;
    ev.xclient.data.l[3] = 1;                     // source: normal application
    XSendEvent(dpy, DefaultRootWindow(dpy), False, SubstructureRedirectMask | SubstructureNotifyMask, &ev);
    XFlush(dpy);
    return 1;
}
static int bw_set_above(uintptr_t w, int on) { return bw_wm_state(w, on, "_NET_WM_STATE_ABOVE"); }
static int bw_set_skip_taskbar(uintptr_t w, int on) {
    int a = bw_wm_state(w, on, "_NET_WM_STATE_SKIP_TASKBAR");
    int b = bw_wm_state(w, on, "_NET_WM_STATE_SKIP_PAGER");
    return a && b;
}
static int bw_get_pos(uintptr_t w, int *x, int *y) {
    GdkSurface *s = bw_surface(w); if (!s) return 0;
    Display *dpy = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
    Window child;
    return XTranslateCoordinates(dpy, gdk_x11_surface_get_xid(s), DefaultRootWindow(dpy), 0, 0, x, y, &child) ? 1 : 0;
}
static int bw_move(uintptr_t w, int x, int y) {
    GdkSurface *s = bw_surface(w); if (!s) return 0;
    Display *dpy = gdk_x11_display_get_xdisplay(gdk_surface_get_display(s));
    XMoveWindow(dpy, gdk_x11_surface_get_xid(s), x, y);
    XFlush(dpy);
    return 1;
}
*/
import "C"

func ptr(w *gtk.Window) C.uintptr_t { return C.uintptr_t(coreglib.InternObject(w).Native()) } // no unsafe.Pointer in Go => go vet clean
func Supported() bool
func SetKeepAbove(w *gtk.Window, on bool) bool   // + runtime.KeepAlive(w)
func SetSkipTaskbar(w *gtk.Window, on bool) bool
func Position(w *gtk.Window) (x, y int, ok bool)
func Move(w *gtk.Window, x, y int) bool
```
`x11win_stub.go` (`//go:build nox11`) has the same API. `Supported` returns false, and everything else returns false or zero values.

`applyWindowHints` (runs 150 ms after map, X11 only):
- If `cfg.Window.X != nil && cfg.Window.Y != nil`, call `Move`. Then, if `AlwaysOnTop`, call `SetKeepAbove(true)` and `SetSkipTaskbar(true)`.
- The widget skips the taskbar only when always-on-top is on. Otherwise a window hidden behind others would be unreachable.
- Before moving, clamp the saved position to the monitor: if x or y is below 0, skip the move.

Behaviour on Wayland: size is restored but position is not, and the always-on-top option is greyed out with the tooltip above. Document this in the README. Users can force XWayland with `GDK_BACKEND=x11 bluetooth-widget`.

`GtkWindow` pointer: `ApplicationWindow` embeds `gtk.Window`, so pass `&w.win.Window`.

---

## 5. Packaging

### 5.1 `data/com.example.BluetoothWidget.desktop`
```
[Desktop Entry]
Type=Application
Name=Bluetooth Widget
GenericName=Bluetooth Devices
Comment=Connect and disconnect paired Bluetooth devices with one click
Exec=bluetooth-widget
Icon=com.example.BluetoothWidget
Terminal=false
Categories=GTK;Utility;Settings;HardwareSettings;
Keywords=bluetooth;headphones;headset;wireless;
StartupNotify=true
```
Validate it with `desktop-file-validate data/com.example.BluetoothWidget.desktop`.

### 5.2 `data/icons/com.example.BluetoothWidget.svg`
```svg
<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
  <rect x="8" y="8" width="112" height="112" rx="28" fill="#3584e4"/>
  <path d="M44 44 L84 80 L64 98 V30 L84 48 L44 84" fill="none" stroke="#fff" stroke-width="9" stroke-linecap="round" stroke-linejoin="round"/>
</svg>
```

### 5.3 Makefile (recipe lines use TABs)
```make
BINARY  := bluetooth-widget
PKG     := ./cmd/bluetooth-widget
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
TAGS    ?= $(shell pkg-config --exists gtk4-x11 x11 2>/dev/null || echo nox11)
LDFLAGS := -s -w -X main.version=$(VERSION)
PURE    := ./internal/bluetooth/... ./internal/config/... ./internal/order/... ./internal/autostart/... ./internal/logging/... ./internal/cli/...

.PHONY: all build run test test-race test-integration vet fmt install uninstall clean
all: build
build:            ; $(GO) build -tags '$(TAGS)' -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)
run: build        ; ./$(BINARY)
test:             ; $(GO) test -tags '$(TAGS)' ./...
test-race:        ; $(GO) test -race $(PURE)
test-integration: ; $(GO) test -tags 'integration $(TAGS)' -v -count=1 ./tests/integration/
vet:              ; $(GO) vet -tags '$(TAGS)' ./...
fmt:              ; gofmt -s -w cmd internal tests
install:          ; ./scripts/install.sh
uninstall:        ; ./scripts/uninstall.sh
clean:            ; rm -f $(BINARY)
```
(Write the recipes on separate lines with a real TAB. The `;` form above is only for brevity.)
- Do **not** add `-trimpath` or `-race` to UI builds. Either one would force a second full gotk4 compile, which takes 10+ minutes.
- `pkg-config --exists gtk4-x11 x11` will succeed once libgtk-4-dev is installed. If it does not, the Makefile falls back to `nox11` automatically.

### 5.4 `scripts/install.sh` (`#!/usr/bin/env bash`, `set -euo pipefail`, never calls sudo)
1. `cd "$(dirname "$0")/.."`.
2. Parse flags: `--autostart` (enable autostart after install) and `--prefix DIR` (default `$HOME/.local`).
3. Prerequisite checks. Each one prints ✓ or ✗; failures print an apt hint (**print only, never run**):
   - `command -v go`, and the Go version ≥ 1.22 (parse `go env GOVERSION`) → hint: install from https://go.dev/dl
   - `command -v gcc` and `command -v pkg-config` → `sudo apt install build-essential pkg-config`
   - `pkg-config --exists 'gtk4 >= 4.6'` → `sudo apt install libgtk-4-dev`
   - `pkg-config --exists gobject-introspection-1.0` → `sudo apt install libgirepository1.0-dev`
   - `pkg-config --exists gtk4-x11 x11` → warning only: "building without X11 always-on-top support"
   - BlueZ check (warning only): `systemctl is-active --quiet bluetooth` **or** `busctl --system status org.bluez >/dev/null 2>&1`. This is only an install-time diagnostic, not a Bluetooth operation.
   - If a required check failed, exit 1 with a combined `sudo apt install ...` line.
4. `make build`. Tell the user the first build may take 5–15 minutes.
5. `install -Dm755 bluetooth-widget "$PREFIX/bin/bluetooth-widget"`.
6. Desktop file: `sed "s|^Exec=.*|Exec=$PREFIX/bin/bluetooth-widget|" data/com.example.BluetoothWidget.desktop` written to `$HOME/.local/share/applications/com.example.BluetoothWidget.desktop`, mode 644. Create the directory with `install -d`.
7. `install -Dm644 data/icons/com.example.BluetoothWidget.svg "$HOME/.local/share/icons/hicolor/scalable/apps/com.example.BluetoothWidget.svg"`.
8. `update-desktop-database "$HOME/.local/share/applications" 2>/dev/null || true` and `gtk-update-icon-cache -q -t -f "$HOME/.local/share/icons/hicolor" 2>/dev/null || true`.
9. If `--autostart` was given: `"$PREFIX/bin/bluetooth-widget" -autostart on`.
10. If `$PREFIX/bin` is not on `$PATH`, print a note.
11. Print "Installed. Launch 'Bluetooth Widget' from the app menu or run: $PREFIX/bin/bluetooth-widget".

### 5.5 `scripts/uninstall.sh`
- Remove the binary, the desktop file, the icon, and `${XDG_CONFIG_HOME:-$HOME/.config}/autostart/com.example.BluetoothWidget.desktop`. Refresh the caches with `|| true`.
- `--purge` also removes `${XDG_CONFIG_HOME:-$HOME/.config}/bluetooth-widget`.
- Print what was removed. Never use sudo.

### 5.6 `scripts/run.sh`
`cd` to the repo root. If the `bluetooth-widget` binary is missing, or any `*.go`/`go.mod` file is newer than it (`find . -name '*.go' -newer bluetooth-widget | grep -q .`), run `make build`. Then `exec ./bluetooth-widget "$@"`.

---

## 6. Tests

All tests below except `ui` and integration are pure Go with no GTK, so they run fast and are race-safe.

`internal/bluetooth/fakebus_test.go`:
```go
type fakeBus struct {
    mu      sync.Mutex
    objs    managedObjects
    objsErr error
    calls   []fakeCall           // {Path, Method}
    sets    []fakeSet            // {Path, Iface, Prop, Value}
    callErr map[string]error     // key: method
    block   chan struct{}        // if non-nil, Call waits on it (to test ErrBusy)
    sig     chan *dbus.Signal
}
```
It implements `bus`. Helpers:
- `propsChanged(path, iface string, changed map[string]dbus.Variant) *dbus.Signal` → `&dbus.Signal{Name: "org.freedesktop.DBus.Properties.PropertiesChanged", Path: path, Body: []any{iface, changed, []string{}}}`
- `ifAdded(...)`, `ifRemoved(...)`, `nameOwner(old, new)`
- `v(x) = dbus.MakeVariant(x)`
- Fixture builder `fixture()`: one adapter `/org/bluez/hci0` (Powered true); 3 devices — paired+connected with Battery1 `byte(82)`; paired+disconnected with Icon absent and Alias `"RK N80 "`; unpaired. Plus one GATT child path that must be ignored.

Required unit tests:
- **device_test.go**: full parse; missing Icon/Name; Alias whitespace trim in DisplayName; fallback to Address; wrong variant types ignored (e.g. `Connected` as a string); `Equal` and `Clone` independence.
- **battery_test.go**: `byte` percentage; `uint32`/`int32` accepted; clamp above 100; labels from child paths; `primaryBattery` rules; ordering of multiple batteries.
- **icons_test.go**: table test for every mapping; unknown and empty values → `[FallbackIcon]`; the last element is always FallbackIcon.
- **errors_test.go**: every `org.bluez.Error.*` from the spec gives the right Kind and a non-empty Friendly/Short text; `Failed` with `"br-connection-page-timeout"` → OutOfRange; `"Blocked through rfkill"` → Rfkill; AlreadyConnected on connect → nil; NotConnected on disconnect → nil; `context.DeadlineExceeded` → Timeout; ServiceUnknown → ServiceUnavailable; both `dbus.Error` and `*dbus.Error` inputs; the Friendly text never contains "org.bluez".
- **store_test.go**: `reset` keeps only paired devices on hci0; unpaired → paired emits Added; paired → unpaired emits Removed; Connected flip emits one Changed; RSSI-only change emits nothing; Battery1 InterfacesAdded sets Battery and emits Changed; InterfacesRemoved(Battery1) clears it; InterfacesRemoved(Device1) emits Removed; a second adapter hci1 appearing does not switch; removing hci0 switches to hci1 with a Reset; device on hci1 is ignored while hci0 is current; adapter Powered change emits AdapterChanged; invalidated props are handled.
- **manager_test.go** (uses `newManager(func() (bus, error) { return fb, nil }, discardLogger)`):
  - Start emits a Reset containing 2 devices.
  - A signal pushed through `fb.sig` reaches the subscriber (collect into a channel; wait ≤1s).
  - `ConnectDevice` records `{path, "org.bluez.Device1.Connect"}`.
  - With `callErr` = `dbus.Error{Name: "org.bluez.Error.Failed", Body: []any{"br-connection-page-timeout"}}` it returns an `*OpError` with Kind OutOfRange.
  - A concurrent second Connect on the same id returns Kind Busy (use `block`).
  - `SetAdapterPowered(true)` records `SetProperty(hci0, "org.bluez.Adapter1", "Powered", true)`.
  - `nameOwner(":1.5", "")` → Reset with `ServiceAvailable == false`; then `nameOwner("", ":1.9")` → reload → Reset with devices.
  - Newbus error → Start returns an error and emits a Reset with the service down.
  - Unknown id → Kind NoDevice.
  - `Close` is idempotent and the loop exits.
- **order_test.go**: connected first, then favourites, then others; alphabetical and case-insensitive within a group; address as tie-breaker; a connected favourite sits in the connected group.
- **config_test.go**: use `t.TempDir()` and `t.Setenv("XDG_CONFIG_HOME", ...)`. Missing file → defaults (ShowBattery and ShowDisconnected true, 340x440). Round-trip. A partial file keeps defaults for missing keys. A corrupt file → defaults plus a non-nil error, and the file is untouched. Save leaves no `.config-*` temp files and the file mode is 0600. Favourites are normalised, uppercased and deduplicated. `SetFavorite` true/false.
- **autostart_test.go**: `Enable` writes the file with the correct `Exec`; `QuoteExec("/a b/c")` == `"\"/a b/c\""`; `Disable` removes it and is idempotent; `IsEnabled` reflects the state.
- **logging_test.go**: `LevelFromEnv` table.
- **tests/integration/bluez_integration_test.go** (`//go:build integration`), **read-only**:
  1. `m := bluetooth.NewSystemManager(slog.Default())`
  2. Start with a 10s ctx. If the error wraps `ErrServiceUnavailable`, skip.
  3. `snap := m.Snapshot()`. Log the adapter and devices. Assert every device has `Paired == true` and a non-empty Address.
  4. `ListPairedDevices()` equals the snapshot length.
  5. Subscribe, sleep 500ms, then Close. Assert no panic and that Close returns within 2s.
  Never call Connect, Disconnect or SetAdapterPowered here.

---

## 7. Phase-by-phase steps and verification

Use this shorthand: `cd /home/farhan.exabyting_bKash.com/Documents/Bkash/bluetooth_widget`. Write the build logs outside the repo, e.g. in your scratchpad.

### Phase 1 — project, D-Bus connection, discovery, parsing
1. `go mod init github.com/shariorfarhan/bluetooth-widget`, then `go get github.com/godbus/dbus/v5@v5.2.2`.
2. Write `internal/logging`, `internal/bluetooth/{events,device,adapter,battery,icons,errors,bus,store,manager}.go`, `internal/config/config.go`, `internal/cli/cli.go` (Dump only), `cmd/bluetooth-widget/main.go` (`-dump`, `-debug`, `-version`), and `.gitignore`.
   It is acceptable to implement the full manager now; Phase 2 then only adds the signal handling and ops.
3. **Pre-warm gotk4 in the background (big time saver).** If `pkg-config --exists 'gtk4 >= 4.6' gobject-introspection-1.0` succeeds, run:
   `go get github.com/diamondburned/gotk4/pkg@v0.2.2` and then **in the background** `go build github.com/diamondburned/gotk4/pkg/gtk/v4 > <scratch>/gotk4-prebuild.log 2>&1`.
   Do **not** run `go mod tidy` until Phase 3; it would drop the unused requirement.
   If pkg-config fails, the dev packages are not installed yet. Skip this and do it at the start of Phase 3.
4. Write device, battery, icons, errors and store tests now (they are cheap).

Verify:
```
gofmt -l . ; go vet ./... ; go test ./internal/...
go build -o bluetooth-widget ./cmd/bluetooth-widget
./bluetooth-widget -dump | jq .
./bluetooth-widget -dump | jq '.devices | length'
busctl --system tree org.bluez --list | grep -cE '/dev_[0-9A-F_]+$'     # compare (all devices here are paired)
for p in $(busctl --system tree org.bluez --list | grep -E '/dev_[0-9A-F_]+$'); do busctl --system get-property org.bluez $p org.bluez.Device1 Alias Paired Connected; done
./bluetooth-widget -dump -debug 2>&1 >/dev/null | head      # slog lines on stderr
```
Expected today: adapter `/org/bluez/hci0` with powered true, and 5 devices whose names match busctl (one alias has a trailing space; `DisplayName` trims it).

### Phase 2 — connect/disconnect and real-time signals
1. Finish the signal loop, NameOwnerChanged handling, `ConnectDevice`/`DisconnectDevice`/`SetAdapterPowered`, and `SubscribeToChanges`.
2. Add `cli.Watch`, `cli.ConnectByAddress` and `cli.SetPower`, with flags `-watch`, `-connect`, `-disconnect` and `-power`.
3. Write `fakebus_test.go` and `manager_test.go`.

Verify:
```
go vet ./... && go test -race ./internal/... && go build -o bluetooth-widget ./cmd/bluetooth-widget
timeout 5 ./bluetooth-widget -watch; echo "exit=$?"      # expect first line kind "reset", exit=124, no errors
```
Manual real-time check (tell the user; **only with their consent**, because it changes real device state):
- Run `./bluetooth-widget -watch`.
- Ask the user to connect or disconnect a device from GNOME Settings, or to switch a paired device on or off.
- Expect `device-changed` lines with `"connected":true/false`, and `"battery"` when Battery1 appears.
- Optionally, with consent: `./bluetooth-widget -connect 2C:BE:EE:79:AC:64` and `-disconnect ...`.

Never use bluetoothctl.

### Phase 3 — GTK4 UI
1. Confirm the dev packages: `pkg-config --modversion gtk4` (expect 4.6.x) and `pkg-config --exists gtk4-x11 x11 gobject-introspection-1.0 && echo ok`.
2. If Phase 1 step 3 was skipped, now run `go get github.com/diamondburned/gotk4/pkg@v0.2.2` and start the background prebuild.
3. Write the `internal/ui/*` files, `x11win` (both files), and update `main.go` (default → `ui.Run`, `runtime.LockOSThread` in `init`).
   In Phase 3 the settings popover can contain only "Show disconnected devices". The other settings come in Phase 4.
4. `go mod tidy`, then check that `grep gotk4 go.mod` shows **v0.2.2**.
5. Build **in the background** with a long wait, because the first cgo build takes 5–15 minutes:
   `go build -v -o bluetooth-widget ./cmd/bluetooth-widget > <scratch>/build.log 2>&1; echo "exit=$?" >> <scratch>/build.log`
   Poll the log. If it runs out of memory, add `-p 2`.

Verify:
```
go vet ./...                                   # after the build (cache is warm)
go test ./internal/...
timeout 6 ./bluetooth-widget 2> <scratch>/run.log; echo "exit=$?"   # expect 124 (still running when killed)
grep -E "CRITICAL|panic|Theme parser" <scratch>/run.log            # expect nothing
(./bluetooth-widget & sleep 3; xwininfo -name Bluetooth | head -5; import -window "$(xwininfo -name Bluetooth | awk '/Window id/{print $4}')" <scratch>/shot.png; pkill -x bluetooth-widget)
```
Look at the screenshot with the Read tool. Expect a header with "Bluetooth" and a gear button, and the 5 rows each showing an icon, name and "○ Click to connect".

If the app exits 0 immediately, another instance is running. Check `pgrep -x bluetooth-widget`.

### Phase 4 — battery, adapter power, favourites, settings, window state
Implement:
- Battery rendering.
- The `off` page with "Turn Bluetooth On".
- The star/favourites toggle, persisted through `config.Store`.
- The full settings popover.
- The error banner with Retry.
- `theme.go`.
- `saveState` and `applyWindowHints` (X11).
- Section headers.
- The Show Battery and Show Disconnected filters.

Verify:
```
go vet ./... && go test ./... && go build -o bluetooth-widget ./cmd/bluetooth-widget
timeout 6 ./bluetooth-widget 2>&1 | grep -E "CRITICAL|panic" ; echo done
cat ~/.config/bluetooth-widget/config.json | jq .        # after toggling a star and closing
```
- Always-on-top check: enable it in settings, then `xprop -name Bluetooth _NET_WM_STATE` should show `_NET_WM_STATE_ABOVE, _NET_WM_STATE_SKIP_TASKBAR, ...`.
- Position check: move the window, close it, reopen it. It should come back at the same spot (X11), and the config has x and y.
- Power check (**only with user consent**): `./bluetooth-widget -power off`. The running widget should switch to "Bluetooth is turned off." within a second, and "Turn Bluetooth On" should restore it.
- Build `go build -tags nox11 -o <scratch>/bw-nox11 ./cmd/bluetooth-widget` to prove the stub compiles.

### Phase 5 — packaging and autostart
Implement:
- `internal/autostart`, the `-autostart` flag, and the settings "Start at login" wiring.
- `data/*`, `scripts/*` (`chmod +x`), and the Makefile.

Verify:
```
make build && make test && make vet
desktop-file-validate data/com.example.BluetoothWidget.desktop
bash -n scripts/install.sh scripts/uninstall.sh scripts/run.sh
make install && ls -l ~/.local/bin/bluetooth-widget ~/.local/share/applications/com.example.BluetoothWidget.desktop
~/.local/bin/bluetooth-widget -autostart on && cat ~/.config/autostart/com.example.BluetoothWidget.desktop
~/.local/bin/bluetooth-widget -autostart off && test ! -e ~/.config/autostart/com.example.BluetoothWidget.desktop && echo removed
make uninstall     # then reinstall if the user wants it installed
```

### Phase 6 — tests and polish
1. Finish the whole test list in §6, including `order`, `config`, `autostart`, `logging` and integration.
2. Polish:
   - Tooltips.
   - Accessible labels.
   - Keyboard: Tab/arrow keys move between rows, Enter/Space activates, Tab reaches the star; Ctrl+Q and Ctrl+W work.
   - No spinner runs while idle.
3. Write the README (§8).

Verify:
```
gofmt -l .            # empty
go vet ./...
go test ./...
go test -race ./internal/bluetooth/... ./internal/config/... ./internal/order/... ./internal/autostart/... ./internal/logging/...
go test -tags integration -v -count=1 ./tests/integration/
make build && timeout 6 ./bluetooth-widget; echo "exit=$?"   # 124
top -b -n 1 -p "$(pgrep -x bluetooth-widget)"   # while running idle: ~0% CPU
```

---

## 8. README outline
1. **Overview.** What it is, a screenshot (optional), and the feature list.
2. **Architecture.** Diagram `GTK4 UI (internal/ui) → BluetoothManager (internal/bluetooth) → D-Bus (godbus) → BlueZ bluetoothd`. Explain the event flow: signals → store → Event → `glib.IdleAdd` → per-row update.
3. **Requirements.**
   - Linux with BlueZ ≥ 5.50 and the system D-Bus.
   - Go ≥ 1.22 (tested with 1.24).
   - GTK ≥ 4.6 dev files: `sudo apt install build-essential pkg-config libgtk-4-dev libgirepository1.0-dev`.
   - Note the gotk4 pin to v0.2.2 and why.
4. **Installation.** The exact commands:
   `git clone <repo> bluetooth-widget && cd bluetooth-widget && make build && make run`, and `make install` (or `./scripts/install.sh --autostart`). Mention the first-build time.
5. **Running.** `./bluetooth-widget`; the CLI flags (`-dump`, `-watch`, `-debug`, `-connect`, `-disconnect`, `-power`, `-autostart`); the `BLUETOOTH_WIDGET_LOG` env variable.
6. **Autostart.** The settings toggle, or `bluetooth-widget -autostart on`; the file location.
7. **Window behaviour and Wayland limitations.** Size is remembered everywhere. Position and always-on-top work on X11 only. On Wayland use the window menu, or `GDK_BACKEND=x11`.
8. **Configuration.** Path, fields, example JSON. Only preferences and MACs are stored.
9. **Troubleshooting.**
   - BlueZ not running: `systemctl status bluetooth`.
   - No adapter: `rfkill`/BIOS; the widget shows "No Bluetooth adapter detected".
   - Blocked by rfkill.
   - Permission errors: the D-Bus policy in `/etc/dbus-1/system.d/bluetooth.conf`, and active-session requirements.
   - GTK build errors: missing `-dev` packages, `could not determine kind of name for C.gtk_…` (wrong gotk4 version), and `-tags nox11`.
   - Connection failures: device off or out of range, re-pairing, and running with `-debug` for D-Bus error names.
10. **Development.** Project layout, `make test`/`test-race`/`test-integration`/`vet`, the fake bus approach, and how to add a Sorter.
11. **Security notes.** No root, no shell-outs for Bluetooth, no credential storage.

---

## 9. Pitfalls to watch for
1. **gotk4 version.** Only `v0.2.2` compiles against GTK 4.6. With v0.3+ or v0.4+ you get `could not determine kind of name for C.gtk_file_dialog_new` and similar errors. Never use `@latest` or `go get -u`.
2. **Build time.** The first gotk4 build takes 5–15 minutes and several GB of RAM. Run it in the background and poll the log. Do not use `-trimpath` or `-race` on packages that import gotk4, because each one causes a separate full rebuild. `go vet` is fast only after `go build` has warmed the cache.
3. **gotk4 API names in v0.2.2.**
   - `gtk.NewApplication(id, gio.ApplicationFlagsNone)`
   - `app.Run(argv) int`
   - `gtk.NewApplicationWindow(app)`
   - `win.SetTitlebar`
   - `gtk.NewBox(gtk.OrientationVertical, 0)`
   - `list.ConnectRowActivated(func(*gtk.ListBoxRow))`
   - `ListBoxSortFunc func(row1, row2 *ListBoxRow) int`
   - `CSSProvider.LoadFromData(string)`
   - `gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), p, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)`
   - `menu.SetPopover(pop)`
   - `stack.AddNamed(child, name)`, `stack.SetVisibleChildName`
   - `spinner.Start()/Stop()`
   - `row.SetHeader(nil)`
   - `win.ConnectCloseRequest(func() bool)`
   - `widget.SetVisible(bool)`. Show and hide are deprecated in some versions; prefer SetVisible.

   Check any method you are unsure of with `grep -n "func (.*) Name(" $(go env GOMODCACHE)/github.com/diamondburned/gotk4/pkg@v0.2.2/gtk/v4/gtk.go`.
4. **`glib.IdleAdd(f interface{})`.** `f` must be `func()` or `func() bool`; returning `true` repeats it. Call GTK only on the main thread. Keep `runtime.LockOSThread()` in `main`'s `init`. Never create widgets before `activate`.
5. **GApplication argv.** Pass only `[]string{os.Args[0]}` to `app.Run`. Otherwise "Unknown option -debug" and similar.
6. **Typed nil into `Widgetter` crashes.** Pass a literal `nil`.
7. **ListBox callbacks get new wrapper objects.** Look rows up with `row.Name()`.
8. **`gio.NewSettings` aborts the process if the schema is missing.** Always check `SettingsSchemaSource.Lookup` first.
9. **dbus.Variant types.**
   - `Percentage` is `byte` (uint8).
   - `Adapter` is `dbus.ObjectPath`.
   - `Icon` and even `Name` may be absent.
   - `Alias` always exists but can have trailing spaces.
   - Use comma-ok assertions everywhere.
   - `GetManagedObjects` decodes into `map[dbus.ObjectPath]map[string]map[string]dbus.Variant`.
10. **Method-error replies are `dbus.Error` by value.** Check both the value and pointer forms with `errors.As`.
11. **GATT and RSSI noise.** Use the arg0-filtered PropertiesChanged match rules, and emit events only when the parsed model actually changed.
12. **InterfacesAdded for a known path.** Merge, don't replace. Battery1 appears separately and later.
13. **Connected can flip before `Connect()` returns.** The row must accept property updates while busy, and `finishOp` re-renders from the latest model.
14. **Timeouts.** Context timeouts end our wait but not BlueZ's operation. Treat `org.bluez.Error.InProgress` as non-fatal. Treat `AlreadyConnected` and `NotConnected` as success.
15. **Subscriber callbacks run on the signal goroutine.** They must never block and never touch GTK directly; only `glib.IdleAdd`. Never emit while holding the manager mutex.
16. **godbus closes the signal channel on `conn.Close()`.** The loop must exit on `!ok`.
17. **`go vet` unsafeptr.** Never write `unsafe.Pointer(someUintptr)` in Go. Pass `C.uintptr_t` to C and cast there, as in the x11win design.
18. **X11 hints.** Apply them only after the window is mapped (map + 150 ms timeout). They are no-ops on Wayland. Grey out the setting with an explanatory tooltip.
19. **`-dump` and other CLI modes must not create a `gtk.Application`.**
20. **Pre-warming.** If you pre-warm gotk4 during Phase 1, do not `go mod tidy` until the UI imports it.
21. **Makefile recipes need TAB characters.** Scripts need `chmod +x`.
22. **Single instance.** A second launch exits 0 immediately. During verification, `pkill -x bluetooth-widget` first.
23. **Never use bluetoothctl, rfkill, hciconfig or sudo** anywhere in the code or scripts. `busctl` appears only in install diagnostics and in your verification commands (read-only, except where the user has agreed).

### Critical Files for Implementation
- /home/farhan.exabyting_bKash.com/Documents/Bkash/bluetooth_widget/internal/bluetooth/store.go
- /home/farhan.exabyting_bKash.com/Documents/Bkash/bluetooth_widget/internal/bluetooth/manager.go
- /home/farhan.exabyting_bKash.com/Documents/Bkash/bluetooth_widget/internal/ui/window.go
- /home/farhan.exabyting_bKash.com/Documents/Bkash/bluetooth_widget/internal/ui/device_row.go
- /home/farhan.exabyting_bKash.com/Documents/Bkash/bluetooth_widget/internal/ui/x11win/x11win.go
- Reference source for API lookups: /home/farhan.exabyting_bKash.com/go/pkg/mod/github.com/diamondburned/gotk4/pkg@v0.2.2/gtk/v4/gtk.go and /home/farhan.exabyting_bKash.com/go/pkg/mod/github.com/godbus/dbus/v5@v5.2.2/
