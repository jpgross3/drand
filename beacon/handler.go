package beacon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	//"github.com/benbjohnson/clock"

	"github.com/drand/drand/log"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber/share"
	clock "github.com/jonboulle/clockwork"
	"google.golang.org/grpc/peer"

	"github.com/drand/drand/key"
	"github.com/drand/drand/net"
)

// Config holds the different cryptographc informations necessary to run the
// randomness beacon.
type Config struct {
	// XXX Think of removing uncessary access to keypair - only given for index
	Private        *key.Pair
	Share          *key.Share
	Group          *key.Group
	Clock          clock.Clock
	WaitBeforeSend time.Duration
	Callback       func(*Beacon)
}

// Handler holds the logic to initiate, and react to the TBLS protocol. Each time
// a full signature can be recosntructed, it saves it to the given Store.
type Handler struct {
	sync.Mutex
	conf *Config
	// to communicate with other drand peers
	client net.ProtocolClient
	// keeps the cryptographic info (group share etc)
	safe *cryptoSafe
	// main logic that treats incoming packet / new beacons created
	chain  *chainStore
	ticker *ticker

	close     chan bool
	addr      string
	started   bool
	stopped   bool
	l         log.Logger
	callbacks *CallbackStore
}

// NewHandler returns a fresh handler ready to serve and create randomness
// beacon
func NewHandler(c net.ProtocolClient, s Store, conf *Config, l log.Logger) (*Handler, error) {
	if conf.Private == nil || conf.Share == nil || conf.Group == nil {
		return nil, errors.New("beacon: invalid configuration")
	}
	idx, exists := conf.Group.Index(conf.Private.Public)
	if !exists {
		return nil, errors.New("beacon: keypair not included in the given group")
	}
	addr := conf.Private.Public.Address()
	// XXX change logging because of resharing
	logger := l.With("index", idx)
	safe := newCryptoSafe()
	safe.SetInfo(conf.Share, conf.Private.Public, conf.Group)
	// genesis block at round 0, next block at round 1
	// THIS is to change when one network wants to build on top of another
	// network's chain. Note that if present it overwrites.
	b := &Beacon{
		Signature: conf.Group.GetGenesisSeed(),
		Round:     0,
	}
	s.Put(b)
	ticker := newTicker(conf.Clock, conf.Group.Period, conf.Group.GenesisTime)
	callbacks := NewCallbackStore(s)
	chain := newChainStore(logger, c, safe, callbacks, ticker)
	handler := &Handler{
		conf:      conf,
		client:    c,
		safe:      safe,
		chain:     chain,
		ticker:    ticker,
		addr:      addr,
		close:     make(chan bool),
		l:         logger,
		callbacks: callbacks,
	}
	return handler, nil
}

var errOutOfRound = "out-of-round beacon request"

// ProcessPartialBeacon receives a request for a beacon partial signature. It
// forwards it to the round manager if it is a valid beacon.
func (h *Handler) ProcessPartialBeacon(c context.Context, p *proto.PartialBeaconPacket) (*proto.Empty, error) {
	peer, _ := peer.FromContext(c)
	addr := peer.Addr.String()
	h.l.Debug("received", "request", "from", addr, "round", p.GetRound(), "prevround", p.GetPreviousRound())

	nextRound, _ := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	currentRound := nextRound - 1

	if p.GetRound() > currentRound {
		h.l.Error("process_partial", addr, "invalid_future_round", p.GetRound(), "current_round", currentRound)
		return nil, fmt.Errorf("invalid round: %d instead of %d", p.GetRound(), currentRound)
	}

	// check that the previous is really a previous round
	if p.GetPreviousRound() != p.GetRound()-1 {
		h.l.Error("process_partial", addr, "invalid_previous_round", p.GetRound(), "partial_previous_round", p.GetPreviousRound())
		return nil, fmt.Errorf("invalid previous round: %d vs current %d", p.GetPreviousRound(), p.GetRound())
	}

	msg := Message(p.GetRound(), p.GetPreviousSig())
	info, err := h.safe.GetInfo(p.GetRound())
	if err != nil {
		h.l.Error("process_partial", addr, "no_info_for_round", p.GetRound())
		return nil, errors.New("no info for this round")
	}

	shortPub := info.pub.Eval(1).V.String()[14:19]
	// verify if request is valid
	if err := key.Scheme.VerifyPartial(info.pub, msg, p.GetPartialSig()); err != nil {
		h.l.Error("process_partial", addr, "err", err, "prev_sig", shortSigStr(p.GetPreviousSig()), "prev_round", p.GetPreviousRound(), "curr_round", currentRound, "msg_sign", shortSigStr(msg), "short_pub", shortPub)
		return nil, err
	}
	idx, _ := key.Scheme.IndexOf(p.GetPartialSig())
	if idx == info.idx {
		h.l.Error("process_partial", addr, "same_index", "got", idx, "our", info.idx, "inadvance_packet?")
		return new(proto.Empty), nil
	}
	h.chain.NewValidPartial(peer.Addr.String(), p)
	return new(proto.Empty), nil
}

// Store returns the store associated with this beacon handler
func (h *Handler) Store() Store {
	return h.chain
}

// Start runs the beacon protocol (threshold BLS signature). The first round
// will sign the message returned by the config.FirstRound() function. If the
// genesis time specified in the group is already passed, Start returns an
// error. In that case, if the group is already running, you should call
// SyncAndRun().
// Round 0 = genesis seed - fixed
// Round 1 starts at genesis time, and is signing over the genesis seed
func (h *Handler) Start() error {
	h.l.Info("beacon", "start")
	if h.conf.Clock.Now().Unix() > h.conf.Group.GenesisTime {
		h.l.Error("genesis_time", "past", "call", "catchup")
		return errors.New("beacon: genesis time already passed. Call Catchup()")
	}
	_, tTime := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	go h.run(tTime)
	return nil
}

// Catchup waits the next round's time to participate. This method is called
// when a node stops its daemon (maintenance or else) and get backs in the
// already running network . If the node does not have the previous randomness,
// it sync its local chain with other nodes to be able to participate in the
// next upcoming round.
func (h *Handler) Catchup() {
	h.chain.RunSync(context.Background())
	_, tTime := NextRound(h.conf.Clock.Now().Unix(), h.conf.Group.Period, h.conf.Group.GenesisTime)
	go h.run(tTime)
}

// Transition makes this beacon continuously sync until the time written in the
// "TransitionTime" in the handler's group file, where he will start generating
// randomness. To sync, he contact the nodes listed in the previous group file
// given.
// TODO: it should be better to use the public streaming API but since it is
// likely to change, right now we use the sync API. Later on when API is well
// defined, best to use streaming.
func (h *Handler) Transition(prevGroup *key.Group) error {
	targetTime := h.conf.Group.TransitionTime
	tRound, tTime := NextRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	// tTime is the time of the next round -
	// we want to compare the actual roudn
	// XXX simplify this by implementing a "RoundOfTime" method
	tTime = tTime - int64(h.conf.Group.Period.Seconds())
	tRound = tRound - 1
	if tTime != targetTime {
		h.l.Fatal("transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
		return nil
	}

	h.safe.SetInfo(nil, h.conf.Private.Public, prevGroup)
	go h.run(targetTime)
	h.chain.RunSync(context.Background())
	return nil
}

func (h *Handler) TransitionNewGroup(newShare *key.Share, newGroup *key.Group) {
	targetTime := newGroup.TransitionTime
	tRound, tTime := NextRound(targetTime, h.conf.Group.Period, h.conf.Group.GenesisTime)
	h.l.Debug("transition", "new_group", "at_round", tRound)
	// tTime is the time of the next round -
	// we want to compare the actual roudn
	// XXX simplify this by implementing a "RoundOfTime" method
	tTime = tTime - int64(h.conf.Group.Period.Seconds())
	tRound = tRound - 1
	if tTime != targetTime {
		h.l.Fatal("transition_time", "invalid_offset", "expected_time", tTime, "got_time", targetTime)
	}
	h.safe.SetInfo(newShare, h.conf.Private.Public, newGroup)
}

// run will wait until it is supposed to start
func (h *Handler) run(startTime int64) {
	chanTick := h.ticker.ChannelAt(startTime)
	h.l.Debug("run_round", "wait", "until", startTime)
	var current roundInfo
	needCatchup := func(b *Beacon) bool {
		if b.Round < current.round {
			return true
		}
		return false
	}
	for {
		select {
		case current = <-chanTick:
			lastBeacon, err := h.chain.Last()
			if err != nil {
				h.l.Error("beacon_loop", "loading_last", "err", err)
				break
			}
			h.l.Debug("beacon_loop", "new_round", "round", current.round, "lastbeacon", lastBeacon.Round)
			h.broadcastNextPartial(lastBeacon)
			if needCatchup(lastBeacon) {
				// We also launch a sync with the other nodes. If there is one node
				// that has a higher beacon, we'll build on it next epoch. If
				// nobody has a higher beacon, then this one will be next if the
				// network conditions allow for it.
				// XXX find a way to start the catchup as soon as the runsync is
				// done. Not critical but leads to faster network recovery.
				h.l.Debug("beacon_loop", "run_sync", "potential_catchup")
				go h.chain.RunSync(context.Background())
			}
		case b := <-h.chain.AppendedBeaconNoSync():
			if needCatchup(b) {
				// When network is down, all alive nodes will broadcast their
				// signatures periodically with the same period. As soon as one
				// new beacon is created,i.e. network is up again, this channel
				// will be triggered and we enter fast mode here.
				// Since that last node is late, nodes must now hurry up to do
				// the next beacons in time -> we run the next beacon now
				// already. If that next beacon is created soon after, this
				// channel will trigger again etc until we arrive at the correct
				// round.
				h.broadcastNextPartial(b)
			}
		case <-h.close:
			h.l.Debug("beacon_loop", "finished")
			return
		}
	}
}

func (h *Handler) broadcastNextPartial(beacon *Beacon) {
	ctx := context.Background()
	// get last beacon at each tick
	last, err := h.chain.Last()
	if err != nil {
		h.l.Error("load_last", "fail", "err", err)
		return
	}
	currentRound := last.Round + 1
	info, err := h.safe.GetInfo(currentRound)
	if err != nil {
		h.l.Error("no_info", currentRound, "BUG")
		return
	}
	if info.share == nil {
		h.l.Error("no_share", currentRound, "BUG", h.safe.String(), "not_synced_yet?")
		return
	}
	msg := Message(currentRound, last.Signature)
	currSig, err := key.Scheme.Sign(info.share.PrivateShare(), msg)
	if err != nil {
		h.l.Fatal("beacon_round", fmt.Sprintf("creating signature: %s", err), "round", currentRound)
		return
	}
	shortPub := info.pub.Eval(1).V.String()[14:19]
	h.l.Debug("start_round", currentRound, "from_sig", shortSigStr(last.Signature), "from_round", last.Round, "msg_sign", shortSigStr(msg), "short_pub", shortPub)
	packet := &proto.PartialBeaconPacket{
		Round:         currentRound,
		PreviousRound: last.Round,
		PreviousSig:   last.Signature,
		PartialSig:    currSig,
	}
	h.chain.NewValidPartial(h.addr, packet)
	for _, id := range info.group.Nodes {
		if info.id.Address() == id.Address() {
			continue
		}
		go func(i *key.Identity) {
			h.l.Debug("beacon_round", currentRound, "send_to", i.Address())
			err := h.client.PartialBeacon(ctx, i, packet)
			if err != nil {
				h.l.Error("beacon_round", currentRound, "err_request", err, "from", i.Address())
				if strings.Contains(err.Error(), errOutOfRound) {
					h.l.Error("beacon_round", currentRound, "node", i.Addr, "reply", "out-of-round")
				}
				return
			}
		}(id)
	}
}

// Stop the beacon loop from aggregating  further randomness, but it
// finishes the one it is aggregating currently.
func (h *Handler) Stop() {
	h.Lock()
	defer h.Unlock()
	if h.stopped {
		return
	}
	close(h.close)
	h.chain.Stop()
	h.ticker.Stop()
	h.stopped = true
	h.l.Info("beacon", "stop")
}

// StopAt will stop the handler at the given time. It is useful when
// transitionining for a resharing.
func (h *Handler) StopAt(stopTime int64) error {
	now := h.conf.Clock.Now().Unix()
	if stopTime <= now {
		// actually we can stop in the present but with "Stop"
		return errors.New("can't stop in the past or present")
	}
	duration := time.Duration(stopTime-now) * time.Second
	h.l.Debug("stop_at", stopTime, "sleep_for", duration.Seconds())
	h.conf.Clock.Sleep(duration)
	h.Stop()
	return nil
}

func (h *Handler) AddCallback(fn func(*Beacon)) {
	h.callbacks.AddCallback(fn)
}

var errOutdatedRound = errors.New("current partial signature not for this round")

func shortSigStr(sig []byte) string {
	max := 3
	if len(sig) < max {
		max = len(sig)
	}
	return hex.EncodeToString(sig[0:max])
}

func shuffleNodes(nodes []*key.Identity) []*key.Identity {
	ids := make([]*key.Identity, 0, len(nodes))
	for _, id := range nodes {
		ids = append(ids, id)
	}
	rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	return ids
}

type cryptoInfo struct {
	group   *key.Group
	share   *key.Share
	pub     *share.PubPoly
	idx     int
	startAt uint64
	id      *key.Identity
}

type cryptoSafe struct {
	sync.Mutex
	infos []*cryptoInfo
}

func newCryptoSafe() *cryptoSafe {
	return &cryptoSafe{}
}

func (c *cryptoSafe) SetInfo(share *key.Share, id *key.Identity, group *key.Group) {
	c.Lock()
	defer c.Unlock()
	info := new(cryptoInfo)
	info.id = id
	info.group = group
	info.pub = group.PublicKey.PubPoly()
	if share != nil {
		info.idx = share.Share.I
		info.share = share
	}
	if group.TransitionTime != 0 {
		time := group.TransitionTime
		nRound, _ := NextRound(time, group.Period, group.GenesisTime)
		info.startAt = nRound - 1
	} else {
		// group started at genesis time
		info.startAt = 0
	}
	c.infos = append(c.infos, info)
	// we sort reverse order so highest round are first
	sort.Slice(c.infos, func(i, j int) bool { return c.infos[i].startAt > c.infos[j].startAt })
}

func (c *cryptoSafe) GetInfo(round uint64) (*cryptoInfo, error) {
	c.Lock()
	defer c.Unlock()
	for _, info := range c.infos {
		if round >= info.startAt {
			return info, nil
		}
	}
	fmt.Printf("failed infos for round %d: %+v\n", round, c.infos)
	return nil, fmt.Errorf("no group info for round %d", round)
}

func (c *cryptoSafe) String() string {
	c.Lock()
	defer c.Unlock()
	var out string
	for _, info := range c.infos {
		out += fmt.Sprintf(" {startAt: %d, sharenil? %v} ", info.startAt, info.share == nil)
	}
	return out
}
