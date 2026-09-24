package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jfreymuth/pulse/proto"
)

// setProfiles fills the anonymous-struct slice in
// proto.GetCardInfoReply.Profiles. Reflection avoids restating that
// struct, whose upstream field tags go vet rejects.
func setProfiles(ci *proto.GetCardInfoReply, ps ...Profile) {
	v := reflect.ValueOf(&ci.Profiles).Elem()
	for _, p := range ps {
		e := reflect.New(v.Type().Elem()).Elem()
		e.FieldByName("Name").SetString(p.Name)
		e.FieldByName("Description").SetString(p.Description)
		e.FieldByName("NumSinks").SetUint(uint64(p.NumSinks))
		e.FieldByName("NumSources").SetUint(uint64(p.NumSources))
		if p.Available {
			e.FieldByName("Available").SetUint(1)
		}
		v.Set(reflect.Append(v, e))
	}
}

const budsMAC = "2C:BE:EE:79:AC:64"

// fakeServer is an in-memory sound server speaking proto request types.
type fakeServer struct {
	mu            sync.Mutex
	card          *proto.GetCardInfoReply
	defaultSink   string
	defaultSource string
	codec         string
	codecAPI      bool
	requests      []string
	fail          map[string]error
	onEvent       func(interface{})
}

func newFakeServer() *fakeServer {
	card := &proto.GetCardInfoReply{
		CardIndex:         7,
		CardName:          "bluez_card.2C_BE_EE_79_AC_64",
		Driver:            "module-bluez5-device.c",
		ActiveProfileName: "a2dp_sink",
		Properties: proto.PropList{
			"device.string": proto.PropListString(budsMAC),
			"device.bus":    proto.PropListString("bluetooth"),
		},
	}
	setProfiles(card,
		Profile{Name: "a2dp_sink", Description: "High Fidelity Playback (A2DP Sink)", NumSinks: 1, Available: true},
		Profile{Name: "handsfree_head_unit", Description: "Handsfree Head Unit (HFP)", NumSinks: 1, NumSources: 1, Available: true},
		Profile{Name: "off", Description: "Off", Available: true},
	)
	return &fakeServer{
		card:          card,
		defaultSink:   "alsa_output.speaker",
		defaultSource: "alsa_input.mic",
		codec:         "sbc",
		codecAPI:      true,
		fail:          map[string]error{},
	}
}

func (f *fakeServer) dial(onEvent func(interface{})) (conn, error) {
	f.mu.Lock()
	f.onEvent = onEvent
	f.mu.Unlock()
	return f, nil
}

func (f *fakeServer) Close() error { return nil }

func (f *fakeServer) sinks() proto.GetSinkInfoListReply {
	out := proto.GetSinkInfoListReply{{SinkName: "alsa_output.speaker", Device: "Speakers", CardIndex: 0}}
	if f.card.ActiveProfileName != "off" {
		profile := "a2dp_sink"
		if f.card.ActiveProfileName != "a2dp_sink" {
			profile = f.card.ActiveProfileName
		}
		out = append(out, &proto.GetSinkInfoReply{
			SinkName: "bluez_sink.2C_BE_EE_79_AC_64." + profile, Device: "CMF Buds Pro 2", CardIndex: 7,
			Properties: proto.PropList{"bluetooth.codec": proto.PropListString(f.codec)},
		})
	}
	return out
}

func (f *fakeServer) sources() proto.GetSourceInfoListReply {
	out := proto.GetSourceInfoListReply{
		{SourceName: "alsa_input.mic", Device: "Internal Mic", CardIndex: 0, MonitorSourceIndex: invalidIndex},
		{SourceName: "alsa_output.speaker.monitor", Device: "Monitor of Speakers", CardIndex: 0, MonitorSourceIndex: 0},
	}
	if f.card.ActiveProfileName == "handsfree_head_unit" {
		out = append(out, &proto.GetSourceInfoReply{
			SourceName: "bluez_source.2C_BE_EE_79_AC_64.handsfree_head_unit", Device: "CMF Buds Pro 2",
			CardIndex: 7, MonitorSourceIndex: invalidIndex,
		})
	}
	return out
}

func (f *fakeServer) Request(req proto.RequestArgs, reply proto.Reply) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := fmt.Sprintf("%T", req)
	f.requests = append(f.requests, name)
	if err := f.fail[name]; err != nil {
		return err
	}
	switch r := req.(type) {
	case *proto.GetServerInfo:
		*reply.(*proto.GetServerInfoReply) = proto.GetServerInfoReply{
			PackageName: "pulseaudio", PackageVersion: "15.99.1",
			DefaultSinkName: f.defaultSink, DefaultSourceName: f.defaultSource,
		}
	case *proto.GetCardInfoList:
		alsa := &proto.GetCardInfoReply{CardIndex: 0, CardName: "alsa_card.pci", Driver: "module-alsa-card.c"}
		card := *f.card // callers read it after the lock is released
		*reply.(*proto.GetCardInfoListReply) = proto.GetCardInfoListReply{alsa, &card}
	case *proto.GetSinkInfoList:
		*reply.(*proto.GetSinkInfoListReply) = f.sinks()
	case *proto.GetSourceInfoList:
		*reply.(*proto.GetSourceInfoListReply) = f.sources()
	case *proto.SetCardProfile:
		if r.CardName != f.card.CardName {
			return proto.Error(5) // no such entity
		}
		f.card.ActiveProfileName = r.ProfileName
	case *proto.SetDefaultSink:
		f.defaultSink = r.SinkName
	case *proto.SetDefaultSource:
		f.defaultSource = r.SourceName
	case *proto.SendObjectMessage:
		if !f.codecAPI || r.ObjectPath != "/card/"+f.card.CardName+"/bluez" {
			return proto.Error(5)
		}
		rep := reply.(*proto.SendObjectMessageReply)
		switch r.Message {
		case "list-codecs":
			if r.Parameters != "" {
				return proto.Error(3)
			}
			rep.Response = `[{"name":"sbc","description":"SBC"},{"name":"sbc_xq_552","description":"SBC XQ 552kbps"}]`
		case "get-codec":
			rep.Response = fmt.Sprintf("%q", f.codec)
		case "switch-codec":
			var c string
			if err := json.Unmarshal([]byte(r.Parameters), &c); err != nil {
				return proto.Error(3)
			}
			f.codec = c
		}
	default:
		return fmt.Errorf("fake: unexpected request %s", name)
	}
	return nil
}

func (f *fakeServer) did(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r == name {
			return true
		}
	}
	return false
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func startFake(t *testing.T, f *fakeServer) *PulseController {
	t.Helper()
	p := newController(quietLogger(), f.dial)
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestSnapshotParsesBluetoothCard(t *testing.T) {
	f := newFakeServer()
	p := startFake(t, f)

	st, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Available || st.Server != "pulseaudio 15.99.1" {
		t.Fatalf("server info not parsed: %+v", st)
	}
	if len(st.Cards) != 1 {
		t.Fatalf("want only the bluez card, got %d cards", len(st.Cards))
	}
	card := st.CardForAddress("2c:be:ee:79:ac:64")
	if card == nil {
		t.Fatal("CardForAddress should match case-insensitively")
	}
	if card.ActiveProfile != "a2dp_sink" || len(card.Profiles) != 3 || !card.Profiles[0].Available {
		t.Fatalf("profiles not parsed: %+v", card)
	}
	if card.CurrentCodec != "sbc" || len(card.Codecs) != 2 || !card.CanSwitchCodec() {
		t.Fatalf("codecs not parsed: %+v", card)
	}
	if len(st.Sources) != 1 || st.Sources[0].Name != "alsa_input.mic" {
		t.Fatalf("monitor sources must be filtered: %+v", st.Sources)
	}
	if got := st.SinksOfCard(7); len(got) != 1 || got[0].Codec != "sbc" {
		t.Fatalf("sink codec prop not parsed: %+v", got)
	}
}

func TestSnapshotWithoutCodecAPI(t *testing.T) {
	f := newFakeServer()
	f.codecAPI = false // e.g. pipewire-pulse
	p := startFake(t, f)

	st, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	card := st.CardForAddress(budsMAC)
	if card == nil || card.Codecs != nil || card.CanSwitchCodec() {
		t.Fatalf("codec list should be empty without the message API: %+v", card)
	}
}

func TestUseDeviceMicrophoneSwitchesToHeadset(t *testing.T) {
	f := newFakeServer()
	p := startFake(t, f)

	if err := p.UseDeviceMicrophone(context.Background(), f.card.CardName); err != nil {
		t.Fatal(err)
	}
	if f.card.ActiveProfileName != "handsfree_head_unit" {
		t.Fatalf("profile = %q, want handsfree_head_unit", f.card.ActiveProfileName)
	}
	if f.defaultSource != "bluez_source.2C_BE_EE_79_AC_64.handsfree_head_unit" {
		t.Fatalf("default source = %q", f.defaultSource)
	}
}

func TestUseDeviceMicrophoneKeepsHeadsetProfile(t *testing.T) {
	f := newFakeServer()
	f.card.ActiveProfileName = "handsfree_head_unit"
	p := startFake(t, f)

	if err := p.UseDeviceMicrophone(context.Background(), f.card.CardName); err != nil {
		t.Fatal(err)
	}
	if f.did("*proto.SetCardProfile") {
		t.Fatal("must not re-set the profile when it already has a microphone")
	}
}

func TestUseDeviceOutputTurnsCardOn(t *testing.T) {
	f := newFakeServer()
	f.card.ActiveProfileName = "off"
	p := startFake(t, f)

	if err := p.UseDeviceOutput(context.Background(), f.card.CardName); err != nil {
		t.Fatal(err)
	}
	if f.card.ActiveProfileName != "a2dp_sink" {
		t.Fatalf("profile = %q, want a2dp_sink", f.card.ActiveProfileName)
	}
	if f.defaultSink != "bluez_sink.2C_BE_EE_79_AC_64.a2dp_sink" {
		t.Fatalf("default sink = %q", f.defaultSink)
	}
}

func TestSwitchCodecSendsJSONString(t *testing.T) {
	f := newFakeServer()
	p := startFake(t, f)

	if err := p.SwitchCodec(context.Background(), f.card.CardName, "sbc_xq_552"); err != nil {
		t.Fatal(err)
	}
	if f.codec != "sbc_xq_552" {
		t.Fatalf("codec = %q", f.codec)
	}
}

func TestSetDefaults(t *testing.T) {
	f := newFakeServer()
	p := startFake(t, f)
	ctx := context.Background()

	if err := p.SetDefaultSink(ctx, "bluez_sink.2C_BE_EE_79_AC_64.a2dp_sink"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetDefaultSource(ctx, "alsa_input.mic"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetProfile(ctx, f.card.CardName, "off"); err != nil {
		t.Fatal(err)
	}
	if f.defaultSink != "bluez_sink.2C_BE_EE_79_AC_64.a2dp_sink" || f.card.ActiveProfileName != "off" {
		t.Fatalf("state not applied: sink=%q profile=%q", f.defaultSink, f.card.ActiveProfileName)
	}
}

func TestOperationErrorsAreWrapped(t *testing.T) {
	f := newFakeServer()
	p := startFake(t, f)

	err := p.SetProfile(context.Background(), "bluez_card.missing", "a2dp_sink")
	var pe proto.Error
	if !errors.As(err, &pe) {
		t.Fatalf("want wrapped proto.Error, got %v", err)
	}
}

func TestNotConnectedIsUnavailable(t *testing.T) {
	p := newController(quietLogger(), func(func(interface{})) (conn, error) {
		return nil, errors.New("no server")
	})
	if err := p.SetDefaultSink(context.Background(), "x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
}

func TestServerEventTriggersPublish(t *testing.T) {
	f := newFakeServer()
	p := startFake(t, f)

	got := make(chan *State, 16)
	unsub := p.Subscribe(func(st *State) { got <- st })
	defer unsub()
	<-got // initial snapshot delivered synchronously

	f.mu.Lock()
	f.card.ActiveProfileName = "handsfree_head_unit"
	onEvent := f.onEvent
	f.mu.Unlock()
	onEvent(&proto.SubscribeEvent{Event: proto.EventCard | proto.EventChange, Index: 7})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case st := <-got:
			if c := st.CardForAddress(budsMAC); c != nil && c.ActiveProfile == "handsfree_head_unit" {
				return
			}
		case <-deadline:
			t.Fatal("no snapshot published after server event")
		}
	}
}

func TestConnectionLossPublishesUnavailable(t *testing.T) {
	f := newFakeServer()
	p := startFake(t, f)

	got := make(chan *State, 16)
	unsub := p.Subscribe(func(st *State) { got <- st })
	defer unsub()
	<-got

	f.mu.Lock()
	onEvent := f.onEvent
	f.mu.Unlock()
	onEvent(&proto.ConnectionClosed{})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case st := <-got:
			if !st.Available {
				return
			}
		case <-deadline:
			t.Fatal("connection loss not published")
		}
	}
}
