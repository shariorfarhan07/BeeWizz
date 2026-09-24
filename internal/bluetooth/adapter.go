package bluetooth

import "github.com/godbus/dbus/v5"

// Adapter is the UI-facing model of an org.bluez.Adapter1 object.
type Adapter struct {
	Path         dbus.ObjectPath `json:"path"`
	Address      string          `json:"address"`
	Name         string          `json:"name"`
	Alias        string          `json:"alias"`
	Powered      bool            `json:"powered"`
	Discoverable bool            `json:"discoverable"`
	Pairable     bool            `json:"pairable"`
}

// Clone returns a copy safe to hand to another goroutine.
func (a *Adapter) Clone() *Adapter {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}

// applyAdapterProps sets only the keys present in props.
func applyAdapterProps(a *Adapter, props map[string]dbus.Variant) {
	if v, ok := props["Address"]; ok {
		if s, ok := v.Value().(string); ok {
			a.Address = s
		}
	}
	if v, ok := props["Name"]; ok {
		if s, ok := v.Value().(string); ok {
			a.Name = s
		}
	}
	if v, ok := props["Alias"]; ok {
		if s, ok := v.Value().(string); ok {
			a.Alias = s
		}
	}
	if v, ok := props["Powered"]; ok {
		if b, ok := v.Value().(bool); ok {
			a.Powered = b
		}
	}
	if v, ok := props["Discoverable"]; ok {
		if b, ok := v.Value().(bool); ok {
			a.Discoverable = b
		}
	}
	if v, ok := props["Pairable"]; ok {
		if b, ok := v.Value().(bool); ok {
			a.Pairable = b
		}
	}
}
