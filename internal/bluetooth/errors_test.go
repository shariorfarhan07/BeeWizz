package bluetooth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestConvertError_AllBluezKinds(t *testing.T) {
	cases := []struct {
		name string
		kind ErrorKind
	}{
		{"org.bluez.Error.Failed", KindFailed},
		{"org.bluez.Error.NotReady", KindNotReady},
		{"org.bluez.Error.NotAvailable", KindNotAvailable},
		{"org.bluez.Error.InProgress", KindInProgress},
		{"org.bluez.Error.AuthenticationCanceled", KindAuthCanceled},
		{"org.bluez.Error.AuthenticationRejected", KindAuthRejected},
		{"org.bluez.Error.AuthenticationFailed", KindAuthFailed},
		{"org.bluez.Error.AuthenticationTimeout", KindAuthTimeout},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			de := dbus.Error{Name: c.name, Body: []interface{}{"details"}}
			err := ConvertError("connect", "Dev", de)
			var oe *OpError
			if !errors.As(err, &oe) {
				t.Fatalf("expected *OpError, got %T: %v", err, err)
			}
			if oe.Kind != c.kind {
				t.Fatalf("kind = %v, want %v", oe.Kind, c.kind)
			}
			if oe.Short() == "" {
				t.Fatalf("Short() empty")
			}
			if strings.Contains(oe.Friendly(), "org.bluez") {
				t.Fatalf("Friendly() leaks dbus name: %q", oe.Friendly())
			}
		})
	}
}

func TestConvertError_FailedWithPageTimeout(t *testing.T) {
	de := dbus.Error{Name: "org.bluez.Error.Failed", Body: []interface{}{"br-connection-page-timeout"}}
	err := ConvertError("connect", "Dev", de)
	var oe *OpError
	if !errors.As(err, &oe) || oe.Kind != KindOutOfRange {
		t.Fatalf("expected KindOutOfRange, got %+v", oe)
	}
}

func TestConvertError_Rfkill(t *testing.T) {
	de := dbus.Error{Name: "org.bluez.Error.Failed", Body: []interface{}{"Blocked through rfkill"}}
	err := ConvertError("connect", "Dev", de)
	var oe *OpError
	if !errors.As(err, &oe) || oe.Kind != KindRfkill {
		t.Fatalf("expected KindRfkill, got %+v", oe)
	}
}

func TestConvertError_BenignOutcomes(t *testing.T) {
	de := dbus.Error{Name: "org.bluez.Error.AlreadyConnected"}
	if err := ConvertError("connect", "Dev", de); err != nil {
		t.Fatalf("connect+AlreadyConnected should be nil, got %v", err)
	}
	de = dbus.Error{Name: "org.bluez.Error.NotConnected"}
	if err := ConvertError("disconnect", "Dev", de); err != nil {
		t.Fatalf("disconnect+NotConnected should be nil, got %v", err)
	}
}

func TestConvertError_ContextDeadline(t *testing.T) {
	err := ConvertError("connect", "Dev", context.DeadlineExceeded)
	var oe *OpError
	if !errors.As(err, &oe) || oe.Kind != KindTimeout {
		t.Fatalf("expected KindTimeout, got %+v", oe)
	}
}

func TestConvertError_ServiceUnknown(t *testing.T) {
	de := dbus.Error{Name: "org.freedesktop.DBus.Error.ServiceUnknown"}
	err := ConvertError("connect", "Dev", de)
	var oe *OpError
	if !errors.As(err, &oe) || oe.Kind != KindServiceUnavailable {
		t.Fatalf("expected KindServiceUnavailable, got %+v", oe)
	}
}

func TestConvertError_ValueAndPointerForms(t *testing.T) {
	de := dbus.Error{Name: "org.bluez.Error.Failed"}
	err1 := ConvertError("connect", "Dev", de)
	err2 := ConvertError("connect", "Dev", &de)
	var oe1, oe2 *OpError
	if !errors.As(err1, &oe1) || !errors.As(err2, &oe2) {
		t.Fatalf("expected both value and pointer *dbus.Error to convert")
	}
	if oe1.Kind != oe2.Kind {
		t.Fatalf("kinds differ: %v vs %v", oe1.Kind, oe2.Kind)
	}
}

func TestConvertError_Nil(t *testing.T) {
	if err := ConvertError("connect", "Dev", nil); err != nil {
		t.Fatalf("nil in -> nil out, got %v", err)
	}
}
