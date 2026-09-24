package bluetooth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

// ErrorKind classifies an operation failure so the UI can pick a
// friendly message without inspecting D-Bus error names itself.
type ErrorKind int

const (
	KindUnknown ErrorKind = iota
	KindFailed
	KindNotReady
	KindNotAvailable
	KindInProgress
	KindAuthCanceled
	KindAuthRejected
	KindAuthFailed
	KindAuthTimeout
	KindOutOfRange
	KindRfkill
	KindTimeout
	KindServiceUnavailable
	KindPermission
	KindBusy
	KindNoDevice
)

var (
	ErrBusy               = errors.New("operation already in progress")
	ErrUnknownDevice      = errors.New("unknown device")
	ErrNoAdapter          = errors.New("no bluetooth adapter")
	ErrServiceUnavailable = errors.New("bluetooth service unavailable")
)

// OpError describes a failed Bluetooth operation with enough detail for
// logging (technical) and for the UI (Short/Friendly).
type OpError struct {
	Op          string
	Device      string
	DBusName    string
	DBusMessage string
	Kind        ErrorKind
	Err         error
}

func (e *OpError) Error() string {
	var b strings.Builder
	if e.Device != "" {
		fmt.Fprintf(&b, "%s %s", e.Op, e.Device)
	} else {
		b.WriteString(e.Op)
	}
	if e.DBusName != "" {
		fmt.Fprintf(&b, ": %s", e.DBusName)
		if e.DBusMessage != "" {
			fmt.Fprintf(&b, ": %s", e.DBusMessage)
		}
	} else if e.Err != nil {
		fmt.Fprintf(&b, ": %s", e.Err.Error())
	}
	return b.String()
}

func (e *OpError) Unwrap() error { return e.Err }

// Short is a compact status suitable for a device row.
func (e *OpError) Short() string {
	switch e.Kind {
	case KindFailed, KindUnknown:
		return "Unable to connect"
	case KindOutOfRange, KindNotAvailable:
		return "Not in range"
	case KindNotReady:
		return "Bluetooth not ready"
	case KindInProgress:
		return "Connecting…"
	case KindAuthCanceled, KindAuthRejected, KindAuthFailed, KindAuthTimeout:
		return "Authentication failed"
	case KindTimeout:
		return "Timed out"
	case KindRfkill:
		return "Blocked"
	case KindPermission:
		return "Permission denied"
	case KindServiceUnavailable:
		return "Bluetooth unavailable"
	case KindBusy:
		return "Busy"
	case KindNoDevice:
		return "Unknown device"
	default:
		return "Unable to connect"
	}
}

// Friendly is a longer message suitable for the error banner. It never
// contains raw D-Bus identifiers.
func (e *OpError) Friendly() string {
	name := e.Device
	verb := "connect to"
	failWord := "connect"
	if e.Op == "disconnect" {
		verb = "disconnect"
		failWord = "disconnect"
	}
	target := func() string {
		if name == "" {
			return ""
		}
		return " " + name
	}()

	switch e.Kind {
	case KindFailed, KindUnknown:
		if e.Op == "set-powered" {
			return "Could not turn Bluetooth on."
		}
		if e.Op == "disconnect" {
			return fmt.Sprintf("Could not disconnect %s.", name)
		}
		return fmt.Sprintf("Could not connect to %s.", name)
	case KindOutOfRange, KindNotAvailable:
		return fmt.Sprintf("Could not %s%s. Make sure it is turned on and nearby.", verb, target)
	case KindNotReady:
		return "Bluetooth is not ready yet. Try again in a moment."
	case KindInProgress:
		return ""
	case KindAuthCanceled, KindAuthRejected, KindAuthFailed, KindAuthTimeout:
		return fmt.Sprintf("Could not connect to %s: authentication failed. Try pairing it again in Settings.", name)
	case KindTimeout:
		return fmt.Sprintf("Connecting to %s timed out.", name)
	case KindRfkill:
		return "Bluetooth is blocked (airplane mode or hardware switch)."
	case KindPermission:
		return "Permission denied by the system Bluetooth service."
	case KindServiceUnavailable:
		return "The Bluetooth service (bluetoothd) is not running."
	case KindBusy:
		return ""
	default:
		if e.Op == "set-powered" {
			return "Could not turn Bluetooth on."
		}
		return fmt.Sprintf("Could not %s%s. (%s)", failWord, target, "unable")
	}
}

// firstString returns body[0].(string) if possible.
func firstString(body []interface{}) string {
	if len(body) == 0 {
		return ""
	}
	if s, ok := body[0].(string); ok {
		return s
	}
	return ""
}

// ConvertError classifies err (typically returned by bus.Call or
// bus.SetProperty) into a *OpError, folding benign results into nil:
//   - connect + AlreadyConnected -> nil
//   - disconnect + NotConnected -> nil
//
// A nil err returns nil.
func ConvertError(op, device string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return &OpError{Op: op, Device: device, Kind: KindTimeout, Err: err}
	}

	if errors.Is(err, ErrBusy) {
		return &OpError{Op: op, Device: device, Kind: KindBusy, Err: err}
	}

	if errors.Is(err, ErrUnknownDevice) {
		return &OpError{Op: op, Device: device, Kind: KindNoDevice, Err: err}
	}

	var name, msg string
	var de dbus.Error
	var dep *dbus.Error
	switch {
	case errors.As(err, &dep):
		name, msg = dep.Name, firstString(dep.Body)
	case errors.As(err, &de):
		name, msg = de.Name, firstString(de.Body)
	}

	if name == "" {
		return &OpError{Op: op, Device: device, Kind: KindUnknown, Err: err}
	}

	// Benign outcomes.
	if op == "connect" && name == "org.bluez.Error.AlreadyConnected" {
		return nil
	}
	if op == "disconnect" && name == "org.bluez.Error.NotConnected" {
		return nil
	}

	kind := classify(name, msg)
	return &OpError{Op: op, Device: device, DBusName: name, DBusMessage: msg, Kind: kind, Err: err}
}

func classify(name, msg string) ErrorKind {
	lowerMsg := strings.ToLower(msg)
	switch name {
	case "org.freedesktop.DBus.Error.ServiceUnknown",
		"org.freedesktop.DBus.Error.NameHasNoOwner":
		return KindServiceUnavailable
	case "org.freedesktop.DBus.Error.NoReply":
		return KindTimeout
	case "org.freedesktop.DBus.Error.AccessDenied":
		return KindPermission
	case "org.bluez.Error.NotAuthorized", "org.bluez.Error.NotPermitted":
		return KindPermission
	case "org.bluez.Error.DoesNotExist":
		return KindNoDevice
	case "org.freedesktop.DBus.Error.UnknownObject":
		return KindNoDevice
	case "org.bluez.Error.NotReady":
		return KindNotReady
	case "org.bluez.Error.NotAvailable":
		return KindNotAvailable
	case "org.bluez.Error.InProgress":
		return KindInProgress
	case "org.bluez.Error.AuthenticationCanceled":
		return KindAuthCanceled
	case "org.bluez.Error.AuthenticationRejected":
		return KindAuthRejected
	case "org.bluez.Error.AuthenticationFailed":
		return KindAuthFailed
	case "org.bluez.Error.AuthenticationTimeout":
		return KindAuthTimeout
	case "org.bluez.Error.Failed":
		if strings.Contains(lowerMsg, "rfkill") {
			return KindRfkill
		}
		if strings.Contains(lowerMsg, "page-timeout") || strings.Contains(lowerMsg, "host is down") {
			return KindOutOfRange
		}
		return KindFailed
	}
	if strings.Contains(lowerMsg, "rfkill") {
		return KindRfkill
	}
	return KindUnknown
}
