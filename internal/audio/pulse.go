package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jfreymuth/pulse/proto"

	"github.com/shariorfarhan/bluetooth-widget/internal/logging"
)

// ErrUnavailable is returned when no sound server connection exists.
var ErrUnavailable = errors.New("audio: sound server not available")

const (
	invalidIndex    = 0xFFFFFFFF
	requestTimeout  = 10 * time.Second
	refreshDebounce = 120 * time.Millisecond
	minBackoff      = time.Second
	maxBackoff      = 30 * time.Second
)

// conn is the part of a sound-server connection the controller uses.
// It exists so tests can substitute a fake server.
type conn interface {
	Request(req proto.RequestArgs, reply proto.Reply) error
	Close() error
}

// dialFunc opens a connection. onEvent receives server-pushed messages
// (*proto.SubscribeEvent, *proto.ConnectionClosed) on a background
// goroutine and must not issue requests itself.
type dialFunc func(onEvent func(msg interface{})) (conn, error)

// PulseController implements Controller for PulseAudio and
// pipewire-pulse.
type PulseController struct {
	log  *slog.Logger
	dial dialFunc

	mu      sync.Mutex
	c       conn
	state   *State
	subs    map[int]func(*State)
	nextSub int
	pending *time.Timer
	closed  chan struct{}
	once    sync.Once

	refreshMu sync.Mutex // serialises snapshots so publishes stay ordered
}

// NewPulseController returns a controller for the user's sound server.
func NewPulseController(log *slog.Logger) *PulseController {
	return newController(log, dialPulse)
}

func newController(log *slog.Logger, dial dialFunc) *PulseController {
	if log == nil {
		log = slog.Default()
	}
	return &PulseController{
		log:    log.With("component", "audio"),
		dial:   dial,
		subs:   map[int]func(*State){},
		closed: make(chan struct{}),
		state:  &State{},
	}
}

type pulseConn struct {
	c  *proto.Client
	nc net.Conn
}

func (p *pulseConn) Request(req proto.RequestArgs, reply proto.Reply) error {
	return p.c.Request(req, reply)
}
func (p *pulseConn) Close() error { return p.nc.Close() }

func dialPulse(onEvent func(interface{})) (conn, error) {
	c, nc, err := proto.Connect("")
	if err != nil {
		return nil, err
	}
	c.Callback = onEvent
	c.SetTimeout(requestTimeout)
	pc := &pulseConn{c: c, nc: nc}

	props := proto.PropList{
		"application.name": proto.PropListString("Bluetooth Widget"),
		"application.id":   proto.PropListString("com.example.BluetoothWidget"),
	}
	if err := c.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		nc.Close()
		return nil, fmt.Errorf("set client name: %w", err)
	}
	mask := proto.SubscriptionMaskSink | proto.SubscriptionMaskSource |
		proto.SubscriptionMaskServer | proto.SubscriptionMaskCard
	if err := c.Request(&proto.Subscribe{Mask: mask}, nil); err != nil {
		nc.Close()
		return nil, fmt.Errorf("subscribe: %w", err)
	}
	return pc, nil
}

// Start implements Controller. It returns after the first connection
// attempt; reconnection continues in the background.
func (p *PulseController) Start(ctx context.Context) error {
	first := make(chan error, 1)
	go p.run(ctx, first)
	return <-first
}

func (p *PulseController) run(ctx context.Context, first chan<- error) {
	backoff := minBackoff
	reported := false
	for {
		lost, err := p.connect()
		if !reported {
			first <- err
			reported = true
		}
		if err != nil {
			p.log.Debug("sound server connect failed", logging.KeyError, err.Error(), "retry_in", backoff.String())
			p.publish(&State{})
			select {
			case <-ctx.Done():
				return
			case <-p.closed:
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		backoff = minBackoff
		p.refresh()

		select {
		case <-ctx.Done():
			p.dropConn()
			return
		case <-p.closed:
			return
		case <-lost:
			p.log.Info("sound server connection lost; reconnecting")
			p.dropConn()
			p.publish(&State{})
		}
	}
}

func (p *PulseController) connect() (<-chan struct{}, error) {
	lost := make(chan struct{})
	var lostOnce sync.Once
	c, err := p.dial(func(msg interface{}) {
		switch msg.(type) {
		case *proto.SubscribeEvent:
			p.scheduleRefresh()
		case *proto.ConnectionClosed:
			lostOnce.Do(func() { close(lost) })
		}
	})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.c = c
	p.mu.Unlock()
	return lost, nil
}

func (p *PulseController) dropConn() {
	p.mu.Lock()
	c := p.c
	p.c = nil
	p.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

func (p *PulseController) conn() (conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.c == nil {
		return nil, ErrUnavailable
	}
	return p.c, nil
}

// Subscribe implements Controller. fn immediately receives the latest
// snapshot.
func (p *PulseController) Subscribe(fn func(*State)) func() {
	p.mu.Lock()
	id := p.nextSub
	p.nextSub++
	p.subs[id] = fn
	st := p.state
	p.mu.Unlock()
	fn(st)
	return func() {
		p.mu.Lock()
		delete(p.subs, id)
		p.mu.Unlock()
	}
}

func (p *PulseController) publish(st *State) {
	p.mu.Lock()
	p.state = st
	subs := make([]func(*State), 0, len(p.subs))
	for _, fn := range p.subs {
		subs = append(subs, fn)
	}
	p.mu.Unlock()
	for _, fn := range subs {
		fn(st)
	}
}

// scheduleRefresh coalesces bursts of server events (a profile switch
// emits many) into one snapshot.
func (p *PulseController) scheduleRefresh() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending != nil {
		return
	}
	p.pending = time.AfterFunc(refreshDebounce, func() {
		p.mu.Lock()
		p.pending = nil
		p.mu.Unlock()
		p.refresh()
	})
}

func (p *PulseController) refresh() {
	p.refreshMu.Lock()
	defer p.refreshMu.Unlock()
	st, err := p.Snapshot(context.Background())
	if err != nil {
		if !errors.Is(err, ErrUnavailable) {
			p.log.Warn("audio snapshot", logging.KeyError, err.Error())
		}
		return
	}
	p.publish(st)
}

// Latest returns the most recently published snapshot.
func (p *PulseController) Latest() *State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Snapshot queries the sound server for a full State.
func (p *PulseController) Snapshot(ctx context.Context) (*State, error) {
	c, err := p.conn()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var info proto.GetServerInfoReply
	if err := c.Request(&proto.GetServerInfo{}, &info); err != nil {
		return nil, fmt.Errorf("get server info: %w", err)
	}
	var cards proto.GetCardInfoListReply
	if err := c.Request(&proto.GetCardInfoList{}, &cards); err != nil {
		return nil, fmt.Errorf("list cards: %w", err)
	}
	var sinks proto.GetSinkInfoListReply
	if err := c.Request(&proto.GetSinkInfoList{}, &sinks); err != nil {
		return nil, fmt.Errorf("list sinks: %w", err)
	}
	var sources proto.GetSourceInfoListReply
	if err := c.Request(&proto.GetSourceInfoList{}, &sources); err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}

	st := &State{
		Available:     true,
		Server:        info.PackageName + " " + info.PackageVersion,
		DefaultSink:   info.DefaultSinkName,
		DefaultSource: info.DefaultSourceName,
	}
	for _, ci := range cards {
		card, ok := parseCard(ci)
		if !ok {
			continue
		}
		if ClassifyProfile(card.ActiveProfile) != ProfileOff {
			card.Codecs, card.CurrentCodec = p.queryCodecs(c, card.Name)
		}
		st.Cards = append(st.Cards, card)
	}
	for _, si := range sinks {
		st.Sinks = append(st.Sinks, Endpoint{
			Name:        si.SinkName,
			Description: si.Device,
			CardIndex:   si.CardIndex,
			Codec:       codecProp(si.Properties),
		})
	}
	for _, si := range sources {
		if isMonitor(si) {
			continue
		}
		st.Sources = append(st.Sources, Endpoint{
			Name:        si.SourceName,
			Description: si.Device,
			CardIndex:   si.CardIndex,
			Codec:       codecProp(si.Properties),
		})
	}
	return st, nil
}

// queryCodecs asks PulseAudio's bluez message handler for the codec
// list. Servers without the handler (pipewire-pulse, PulseAudio < 15)
// return an error, which just means "no codec choice".
func (p *PulseController) queryCodecs(c conn, card string) ([]Codec, string) {
	path := codecObjectPath(card)
	var list proto.SendObjectMessageReply
	if err := c.Request(&proto.SendObjectMessage{ObjectPath: path, Message: "list-codecs"}, &list); err != nil {
		p.log.Debug("list-codecs unsupported", "card", card, logging.KeyError, err.Error())
		return nil, ""
	}
	codecs, err := ParseCodecList(list.Response)
	if err != nil {
		p.log.Debug("list-codecs parse", "card", card, logging.KeyError, err.Error())
		return nil, ""
	}
	var cur proto.SendObjectMessageReply
	current := ""
	if err := c.Request(&proto.SendObjectMessage{ObjectPath: path, Message: "get-codec"}, &cur); err == nil {
		current = ParseCurrentCodec(cur.Response)
	}
	return codecs, current
}

func codecObjectPath(card string) string { return "/card/" + card + "/bluez" }

// ParseCodecList parses the list-codecs JSON response:
// [{"name":"sbc","description":"SBC"}, ...].
func ParseCodecList(resp string) ([]Codec, error) {
	var raw []Codec
	if err := json.Unmarshal([]byte(resp), &raw); err != nil {
		return nil, err
	}
	out := raw[:0]
	for _, c := range raw {
		if c.Name == "" {
			continue
		}
		if c.Description == "" {
			c.Description = strings.ToUpper(c.Name)
		}
		out = append(out, c)
	}
	return out, nil
}

// ParseCurrentCodec parses the get-codec response: a JSON string or null.
func ParseCurrentCodec(resp string) string {
	var s *string
	if err := json.Unmarshal([]byte(resp), &s); err != nil || s == nil {
		return ""
	}
	return *s
}

var macInName = regexp.MustCompile(`([0-9A-Fa-f]{2}[_:]){5}[0-9A-Fa-f]{2}`)

func parseCard(ci *proto.GetCardInfoReply) (Card, bool) {
	props := ci.Properties
	isBT := strings.Contains(ci.Driver, "bluez") || prop(props, "device.bus") == "bluetooth" ||
		strings.HasPrefix(ci.CardName, "bluez_card.")
	if !isBT {
		return Card{}, false
	}
	addr := prop(props, "api.bluez5.address")
	if addr == "" {
		addr = prop(props, "device.string")
	}
	if !macInName.MatchString(addr) {
		addr = strings.ReplaceAll(macInName.FindString(ci.CardName), "_", ":")
	}
	if addr == "" {
		return Card{}, false
	}
	card := Card{
		Index:         ci.CardIndex,
		Name:          ci.CardName,
		Address:       strings.ToUpper(addr),
		ActiveProfile: ci.ActiveProfileName,
	}
	for _, pr := range ci.Profiles {
		card.Profiles = append(card.Profiles, Profile{
			Name:        pr.Name,
			Description: pr.Description,
			Available:   pr.Available != 0,
			NumSinks:    int(pr.NumSinks),
			NumSources:  int(pr.NumSources),
		})
	}
	return card, true
}

func prop(pl proto.PropList, key string) string {
	if v, ok := pl[key]; ok {
		return v.String()
	}
	return ""
}

func codecProp(pl proto.PropList) string {
	if v := prop(pl, "bluetooth.codec"); v != "" {
		return v
	}
	return prop(pl, "api.bluez5.codec")
}

func isMonitor(si *proto.GetSourceInfoReply) bool {
	// For sources, the MonitorSource* fields hold the sink this source
	// monitors ("monitor_of_sink" in libpulse).
	return si.MonitorSourceIndex != invalidIndex || prop(si.Properties, "device.class") == "monitor"
}

// SetProfile implements Controller.
func (p *PulseController) SetProfile(ctx context.Context, card, profile string) error {
	return p.do(ctx, "set_profile", []any{"card", card, "profile", profile}, func(c conn) error {
		return c.Request(&proto.SetCardProfile{CardIndex: invalidIndex, CardName: card, ProfileName: profile}, nil)
	})
}

// SetDefaultSink implements Controller.
func (p *PulseController) SetDefaultSink(ctx context.Context, sink string) error {
	return p.do(ctx, "set_default_sink", []any{"sink", sink}, func(c conn) error {
		return c.Request(&proto.SetDefaultSink{SinkName: sink}, nil)
	})
}

// SetDefaultSource implements Controller.
func (p *PulseController) SetDefaultSource(ctx context.Context, source string) error {
	return p.do(ctx, "set_default_source", []any{"source", source}, func(c conn) error {
		return c.Request(&proto.SetDefaultSource{SourceName: source}, nil)
	})
}

// UseDeviceMicrophone implements Controller.
func (p *PulseController) UseDeviceMicrophone(ctx context.Context, card string) error {
	return p.do(ctx, "use_microphone", []any{"card", card}, func(c conn) error {
		return useCardEndpoint(c, card, true)
	})
}

// UseDeviceOutput implements Controller.
func (p *PulseController) UseDeviceOutput(ctx context.Context, card string) error {
	return p.do(ctx, "use_output", []any{"card", card}, func(c conn) error {
		return useCardEndpoint(c, card, false)
	})
}

// useCardEndpoint makes the card's source (input) or sink (output) the
// default, first switching to a profile that provides one if needed:
// headset mode for a microphone, high-quality mode (falling back to
// headset) for output.
func useCardEndpoint(c conn, card string, input bool) error {
	var cards proto.GetCardInfoListReply
	if err := c.Request(&proto.GetCardInfoList{}, &cards); err != nil {
		return fmt.Errorf("list cards: %w", err)
	}
	var target *Card
	for _, ci := range cards {
		if ci.CardName == card {
			if parsed, ok := parseCard(ci); ok {
				target = &parsed
			}
		}
	}
	if target == nil {
		return fmt.Errorf("card %s not found", card)
	}

	has := func(p *Profile) bool {
		if p == nil {
			return false
		}
		if input {
			return p.NumSources > 0
		}
		return p.NumSinks > 0
	}
	if !has(target.Active()) {
		var want *Profile
		if input {
			want = target.BestProfile(ProfileHeadset)
		} else {
			want = target.BestProfile(ProfileHighQuality)
			if !has(want) {
				want = target.BestProfile(ProfileHeadset)
			}
		}
		if !has(want) {
			return fmt.Errorf("card %s has no suitable profile", card)
		}
		req := &proto.SetCardProfile{CardIndex: invalidIndex, CardName: card, ProfileName: want.Name}
		if err := c.Request(req, nil); err != nil {
			return fmt.Errorf("switch to %s: %w", want.Name, err)
		}
	}

	if input {
		var sources proto.GetSourceInfoListReply
		if err := c.Request(&proto.GetSourceInfoList{}, &sources); err != nil {
			return fmt.Errorf("list sources: %w", err)
		}
		for _, si := range sources {
			if si.CardIndex == target.Index && !isMonitor(si) {
				return c.Request(&proto.SetDefaultSource{SourceName: si.SourceName}, nil)
			}
		}
		return fmt.Errorf("card %s exposes no microphone", card)
	}
	var sinks proto.GetSinkInfoListReply
	if err := c.Request(&proto.GetSinkInfoList{}, &sinks); err != nil {
		return fmt.Errorf("list sinks: %w", err)
	}
	for _, si := range sinks {
		if si.CardIndex == target.Index {
			return c.Request(&proto.SetDefaultSink{SinkName: si.SinkName}, nil)
		}
	}
	return fmt.Errorf("card %s exposes no output", card)
}

// SwitchCodec implements Controller. The switch completes
// asynchronously in the server; the resulting card change event
// triggers a fresh snapshot.
func (p *PulseController) SwitchCodec(ctx context.Context, card, codec string) error {
	return p.do(ctx, "switch_codec", []any{"card", card, "codec", codec}, func(c conn) error {
		params, _ := json.Marshal(codec)
		req := &proto.SendObjectMessage{ObjectPath: codecObjectPath(card), Message: "switch-codec", Parameters: string(params)}
		return c.Request(req, &proto.SendObjectMessageReply{})
	})
}

func (p *PulseController) do(ctx context.Context, op string, attrs []any, fn func(conn) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := p.conn()
	if err != nil {
		return err
	}
	p.log.Info(op, append([]any{logging.KeyOp, op}, attrs...)...)
	if err := fn(c); err != nil {
		p.log.Error(op, append([]any{logging.KeyOp, op, logging.KeyError, err.Error()}, attrs...)...)
		p.scheduleRefresh()
		return fmt.Errorf("%s: %w", op, err)
	}
	p.scheduleRefresh()
	return nil
}

// Close implements Controller.
func (p *PulseController) Close() error {
	p.once.Do(func() {
		close(p.closed)
		p.mu.Lock()
		if p.pending != nil {
			p.pending.Stop()
		}
		p.mu.Unlock()
		p.dropConn()
	})
	return nil
}
