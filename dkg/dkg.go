package dkg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	dkg_proto "github.com/drand/drand/protobuf/crypto/dkg"
	vss_proto "github.com/drand/drand/protobuf/crypto/vss"
	"github.com/drand/kyber"
	dkg "github.com/drand/kyber/share/dkg/pedersen"
	vss "github.com/drand/kyber/share/vss/pedersen"
	clock "github.com/jonboulle/clockwork"
	"google.golang.org/grpc/peer"
)

// Suite is the suite used by the crypto dkg package
type Suite = dkg.Suite

// DefaultTimeout is the timeout used by default when unspecified in the config
const DefaultTimeout = time.Duration(1) * time.Minute

// Config holds all necessary information to run a dkg protocol. This config is
// transformed to be passed down to the kyber dkg library.
type Config struct {
	Suite          Suite
	Key            *key.Pair
	NewNodes       *key.Group
	OldNodes       *key.Group
	Share          *key.Share
	Timeout        time.Duration
	Reader         io.Reader
	UserReaderOnly bool
	Clock          clock.Clock
}

// Share represents the private information that a node holds after a successful
// DKG. This information MUST stay private !
type Share = dkg.DistKeyShare

// Handler is the stateful struct that runs a DKG with the peers
type Handler struct {
	net           Network     // network to send data out
	conf          *Config     // configuration given at init time
	cdkg          *dkg.Config // dkg config
	private       *key.Pair   // private key
	nidx          int         // the index of the private/public key pair in the new list
	oidx          int
	newNode       bool                       // true if this node belongs in the new group or not
	oldNode       bool                       // true if this node belongs to the oldNode list
	state         *dkg.DistKeyGenerator      // dkg stateful struct
	n             int                        // number of participants
	tmpResponses  map[uint32][]*dkg.Response // temporary buffer of responses
	sentDeals     bool                       // true if the deals have been sent already
	dealProcessed int                        // how many deals have we processed so far
	respProcessed int                        // how many responses have we processed so far
	done          bool                       // is the protocol done
	shareCh       chan Share                 // share gets sent over shareCh when ready
	errCh         chan error                 // any fatal error for the protocol gets sent over
	exitCh        chan bool                  // any old node not in the new group will signal the end of the protocol through this channel

	sync.Mutex
	share           *dkg.DistKeyShare // the final share generated
	sendDeal        bool              // true if this DKG should be expected to send a deal
	timerCh         chan bool         // closed when timer should stop waiting
	timeouted       bool              // true if timeout occured
	timeoutLaunched bool              // true if timeout has launched already
	l               log.Logger
}

// NewHandler returns a fresh dkg handler using this private key.
func NewHandler(n Network, c *Config, l log.Logger) (*Handler, error) {
	if c.Clock == nil {
		return nil, errors.New("dkg: handler needs at least a Clock")
	}
	var share *dkg.DistKeyShare
	if c.Share != nil {
		s := dkg.DistKeyShare(*c.Share)
		share = &s
	}
	var dpub []kyber.Point
	if c.OldNodes != nil && c.OldNodes.PublicKey != nil {
		dpub = c.OldNodes.PublicKey.Coefficients
	}

	if c.Timeout == time.Duration(0) {
		c.Timeout = DefaultTimeout
	}
	cdkg := &dkg.Config{
		Suite:          c.Suite.(dkg.Suite),
		Longterm:       c.Key.Key,
		NewNodes:       c.NewNodes.Points(),
		PublicCoeffs:   dpub,
		Share:          share,
		Threshold:      c.NewNodes.Threshold,
		Reader:         c.Reader,
		UserReaderOnly: c.UserReaderOnly,
	}

	if c.OldNodes != nil {
		cdkg.OldNodes = c.OldNodes.Points()
		cdkg.OldThreshold = c.OldNodes.Threshold
	}
	state, err := dkg.NewDistKeyHandler(cdkg)
	if err != nil {
		return nil, fmt.Errorf("dkg: error using dkg library: %s", err)
	}

	var newNode, oldNode bool
	var nidx, oidx int
	var found bool
	nidx, found = c.NewNodes.Index(c.Key.Public)
	if found {
		newNode = true
	}
	if c.OldNodes != nil {
		oidx, found = c.OldNodes.Index(c.Key.Public)
		if found {
			oldNode = true
		}
	}
	var shouldSendDeal bool
	if newNode && c.OldNodes == nil {
		// fresh dkg case
		shouldSendDeal = true
	} else if oldNode && c.OldNodes != nil {
		// resharing case
		shouldSendDeal = true
	}
	handler := &Handler{
		conf:         c,
		cdkg:         cdkg,
		private:      c.Key,
		state:        state,
		net:          n,
		nidx:         nidx,
		oidx:         oidx,
		newNode:      newNode,
		oldNode:      oldNode,
		tmpResponses: make(map[uint32][]*dkg.Response),
		n:            len(cdkg.NewNodes),
		shareCh:      make(chan Share, 1),
		errCh:        make(chan error, 1),
		exitCh:       make(chan bool, 1),
		sendDeal:     shouldSendDeal,
		timerCh:      make(chan bool, 1),
	}
	handler.l = l.With("dkg", handler.info())
	return handler, nil
}

// Process process an incoming message from the network.
func (h *Handler) Process(c context.Context, packet *dkg_proto.Packet) {
	h.Lock()
	defer h.Unlock()
	if !h.timeoutLaunched {
		h.timeoutLaunched = true
		go h.startTimer() // start timer at the first message received
	}
	peer, _ := peer.FromContext(c)
	switch {
	case packet.Deal != nil:
		h.processDeal(peer, packet.Deal)
	case packet.Response != nil:
		h.processResponse(peer, packet.Response)
	case packet.Justification != nil:
		panic("not yet implemented")
	}
}

// Start sends the first message to run the protocol
func (h *Handler) Start() {
	h.Lock()
	if !h.timeoutLaunched {
		h.timeoutLaunched = true
		go h.startTimer() // start timer at the first message received
	}
	h.Unlock()
	if err := h.sendDeals(); err != nil {
		h.errCh <- err
	}
}

// WaitShare returns a channel over which the share will be sent over when
// ready.
func (h *Handler) WaitShare() chan Share {
	return h.shareCh
}

// WaitError returns a channel over which any fatal error for the protocol is
// sent to.
func (h *Handler) WaitError() chan error {
	return h.errCh
}

// WaitExit returns a channel which is signalled over when a node that is
// leaving a group, i.e. public key only present in the old list of nodes, has
// seen all necessary responses to attest the validity of the new deals.
func (h *Handler) WaitExit() chan bool {
	return h.exitCh
}

// QualifiedGroup returns the group that correctly finished running the DKG
// protocol. It may be a subset of the group given in the NewNodes field in the
// config. Indeed, not all members may have been online or have completed the
// protocol sucessfully. This group must be saved to be re-used later on
// in case of a renewal for the share.
// This method MUST only be called if the dkg has finished as signalled on the
// `WaitShare` channel.
// XXX Best to group that with the WaitShare channel.
func (h *Handler) QualifiedGroup() *key.Group {
	sharesIndex := h.state.QualifiedShares()
	newGroup := make([]*key.Identity, 0, len(sharesIndex))
	ids := h.conf.NewNodes.Identities()
	var addresses []string
	for _, idx := range sharesIndex {
		newGroup = append(newGroup, ids[idx])
		addresses = append(addresses, ids[idx].Address())
	}
	addr := "[" + strings.Join(addresses, ",") + "]"
	h.l.Info("qualified_idx", intArray(sharesIndex), "qual_addresses", addr)
	return key.LoadGroup(newGroup, &key.DistPublic{Coefficients: h.share.Commits}, h.conf.NewNodes.Threshold)
}

func (h *Handler) startTimer() {
	fmt.Printf(" DKG HANDLER TIMEOUT %s -> now %d -> will trigger at %d\n", h.conf.Key.Public.Address(), h.conf.Clock.Now().Unix(), h.conf.Clock.Now().Add(h.conf.Timeout).Unix())
	select {
	case <-h.conf.Clock.After(h.conf.Timeout):
		h.Lock()
		defer h.Unlock()
		fmt.Printf("DKG HANDLER %s - %d - timeout triggered !\n", h.conf.Key.Public.Address(), h.nidx)
		h.l.Info("timout", "triggered")
		h.timeouted = true
		h.state.SetTimeout()
		h.checkCertified()
	case <-h.timerCh:
		// no need to set the timeout, i.e. we have all the required deals and
		// responses !
		return
	}
}

func (h *Handler) processDeal(p *peer.Peer, pdeal *dkg_proto.Deal) {
	localLog := h.l.With("process", "deal")
	h.dealProcessed++
	deal := &dkg.Deal{
		Index:     pdeal.Index,
		Signature: pdeal.Signature,
		Deal: &vss.EncryptedDeal{
			DHKey:     pdeal.Deal.Dhkey,
			Signature: pdeal.Deal.Signature,
			Nonce:     pdeal.Deal.Nonce,
			Cipher:    pdeal.Deal.Cipher,
		},
	}
	defer h.processTmpResponses(deal)
	localLog.Debug("deal_from", h.dealerAddr(deal.Index), "processed", h.dealProcessed, "sent", h.sentDeals)
	resp, err := h.state.ProcessDeal(deal)
	if err != nil {
		localLog.Error("kyber", err)
		return
	}

	if !h.sentDeals && h.sendDeal {
		localLog.Debug("action", "sending_deals")
		go func() {
			if err := h.sendDeals(); err != nil {
				h.errCh <- err
			}
		}()
	}

	if h.newNode {
		// this should always be the case since that function should only be
		// called  to new nodes members§
		out := &dkg_proto.Packet{
			Response: &dkg_proto.Response{
				Index: resp.Index,
				Response: &vss_proto.Response{
					SessionId: resp.Response.SessionID,
					Index:     resp.Response.Index,
					Status:    resp.Response.Status,
					Signature: resp.Response.Signature,
				},
			},
		}
		localLog.Debug("action", "broadcasting_responses")
		go h.broadcast(out, true, "response")
	}
}

func (h *Handler) processTmpResponses(deal *dkg.Deal) {
	defer h.checkCertified()
	resps, ok := h.tmpResponses[deal.Index]
	if !ok {
		return
	}
	h.l.Debug("process_tmp", "dealer", deal.Index, "tmp_responses", len(resps))
	delete(h.tmpResponses, deal.Index)
	for _, r := range resps {
		_, err := h.state.ProcessResponse(r)
		if err != nil {
			h.l.Error("process_tmp", err)
		}
	}
}

func (h *Handler) processResponse(p *peer.Peer, presp *dkg_proto.Response) {
	defer h.checkCertified()
	localLog := h.l.With("process", "response")
	//h.l.Debug("process_deal", deal.Index, "from", h.dealerAddr(deal.Index),
	h.respProcessed++

	resp := &dkg.Response{
		Index: presp.Index,
		Response: &vss.Response{
			SessionID: presp.Response.SessionId,
			Index:     presp.Response.Index,
			Status:    presp.Response.Status,
			Signature: presp.Response.Signature,
		},
	}
	j, err := h.state.ProcessResponse(resp)
	localLog.Debug("from", resp.Response.Index, "for_deal", resp.Index, "addr", p.Addr.String())
	if err != nil {
		if err == vss.ErrNoDealBeforeResponse {
			h.tmpResponses[resp.Index] = append(h.tmpResponses[resp.Index], resp)
			localLog.Debug("response_unknown_deal", resp.Index, "addr", p.Addr.String())
			return
		}
		localLog.Error("for_deal", resp.Index, "addr", p.Addr, "error", err)
		return
	}
	if j != nil && h.oldNode {
		// XXX TODO
		localLog.Debug("broadcasting justification")
		packet := &dkg_proto.Packet{
			Justification: &dkg_proto.Justification{
				Index: j.Index,
				Justification: &vss_proto.Justification{
					SessionId: j.Justification.SessionID,
					Index:     j.Justification.Index,
					Signature: j.Justification.Signature,
				},
			},
		}
		go h.broadcast(packet, true, "justification")
	}

	localLog.Debug("processed_resp", h.respProcessed, "processed_total", h.n*(h.n-1), "certified", h.state.Certified())

}

func (h *Handler) info() string {
	var s string
	if h.oldNode {
		s += fmt.Sprintf("(%d ", h.oidx)
	} else {
		s += fmt.Sprintf("( -- ")
	}
	if h.newNode {
		s += fmt.Sprintf(", %d)", h.nidx)
	} else {
		s += fmt.Sprintf(", --)")
	}
	return s
}

// checkCertified checks if there has been enough responses and if so, creates
// the distributed key share, and sends it along the channel returned by
// WaitShare.
func (h *Handler) checkCertified() {
	if h.done {
		h.l.Debug("certified", "early_return")
		return
	}
	var fully = true
	if !h.state.Certified() {
		// we miss some responses / deals
		if !(h.state.ThresholdCertified() && h.timeouted) {
			// if it's not threshold-certified or the timeout did not occur,
			// that means it's not finished yet. After timeout, we are ready to
			// accept the threshold-certified deals.
			return
		}
		// we have enough deals/responses and the timeout passed so we consider
		// it the end of the protocol
		fully = false
	}
	h.done = true
	close(h.timerCh)
	if !h.newNode {
		// we just signal an empty message since we are not holder of a share
		// anymore
		h.exitCh <- true
		return
	}

	dks, err := h.state.DistKeyShare()
	if err != nil {
		h.l.Error("certified", "err getting share", err)
		return
	}
	share := Share(*dks)
	h.share = &share
	t := "threshold"
	if fully {
		t = "fully"
	}
	hash := sha256.New()
	buff, _ := share.PriShare().V.MarshalBinary()
	hash.Write(buff)
	h.l.Info("certified", t, "share_hash", hex.EncodeToString(hash.Sum(nil)[0:3]))
	h.shareCh <- share
}

// sendDeals tries to send the deals to each of the nodes.
// It returns an error if a number of node superior to the threshold have not
// received the deal. It is basically a no-go.
func (h *Handler) sendDeals() error {
	h.Lock()
	if h.sentDeals == true {
		h.Unlock()
		return nil
	}
	h.sentDeals = true
	deals, err := h.state.Deals()
	if err != nil {
		h.Unlock()
		return err
	}
	h.Unlock()
	h.l.Debug("send_deal", "start")
	statusCh := make(chan bool, len(deals))
	ids := h.conf.NewNodes.Identities()
	for i, deal := range deals {
		if i == h.nidx && h.newNode {
			h.l.Fatal("same index deal", i, "pubkey", h.conf.Key.Public.Key.String())
			panic("this is a bug with drand that should not happen. Please submit report if possible")
		}
		go func(i int, deal *dkg.Deal) {
			id := ids[i]
			packet := &dkg_proto.Packet{
				Deal: &dkg_proto.Deal{
					Index:     deal.Index,
					Signature: deal.Signature,
					Deal: &vss_proto.EncryptedDeal{
						Dhkey:     deal.Deal.DHKey,
						Signature: deal.Deal.Signature,
						Nonce:     deal.Deal.Nonce,
						Cipher:    deal.Deal.Cipher,
					},
				},
			}
			h.l.Debug("send_deal_to", i, "addr", id.Address())
			if err := h.net.Send(id, packet); err != nil {
				h.l.Error("send_deal_fail", err, "to", id.Address())
				statusCh <- false
			} else {
				statusCh <- true
			}
		}(i, deal)
	}

	var good = 1
	var bad = 0
	for {
		if <-statusCh {
			good++
		} else {
			bad++
		}
		if bad > h.conf.NewNodes.Threshold {
			return fmt.Errorf("dkg: error sending deals to %d  nodes / %d (threshold %d)", bad, h.n, h.conf.NewNodes.Threshold)
		}
		if bad+good == h.conf.NewNodes.Len() {
			h.l.Info("send_deal", "sucess", "to", good-1)
			return nil
		}
	}

}

// The following packets must be sent to the following nodes:
// - Deals are sent to the new nodes only
// - Responses are sent to to both new nodes and old nodes but *only once per
// node*
// - Justification are sent to the new nodes only
func (h *Handler) broadcast(p *dkg_proto.Packet, toOldNodes bool, msgType string) {
	var sent = make(map[string]bool)
	var good, oldGood int
	for i, id := range h.conf.NewNodes.Identities() {
		if toOldNodes {
			sent[id.Key.String()] = true
		}
		if h.newNode && h.nidx == i {
			continue
		}
		if err := h.net.Send(id, p); err != nil {
			h.l.Error("broadcast", err, "to", id.Address(), "type", msgType)
			continue
		}
		h.l.Debug("broadcast", "sucess", "to", id.Address(), "type", msgType)
		good++
	}

	if toOldNodes && h.conf.OldNodes != nil {
		for _, id := range h.conf.OldNodes.Identities() {
			// don't send twice to same address
			_, present := sent[id.Key.String()]
			if present {
				continue
			}
			if err := h.net.Send(id, p); err != nil {
				h.l.Debug("broadcast_old", err, "to", id.Address(), "type", msgType)
				continue
			}
			h.l.Debug("broadcast_old", "sucess", "to", id.Address(), "type", msgType)
			oldGood++
		}

	}
	h.l.Debug("broadcast", "done", "type", msgType)
}

func (h *Handler) addr() string {
	return h.private.Public.Address()
}

func (h *Handler) dealerAddr(i uint32) string {
	defer func() {
		if err := recover(); err != nil {
			h.l.Fatal("dealer_addr", err, "oldnodes", h.conf.OldNodes)
		}
	}()
	if h.conf.OldNodes == nil {
		return h.conf.NewNodes.Public(int(i)).Address()
	}
	return h.conf.OldNodes.Public(int(i)).Address()

}

func (h *Handler) raddr(i uint32, oldNodes bool) string {
	defer func() {
		if err := recover(); err != nil {
			h.l.Fatal("remote_addr", err, "oldnodes", h.conf.OldNodes)
		}
	}()
	if oldNodes {
		return h.conf.OldNodes.Public(int(i)).Address()
	}
	return h.conf.NewNodes.Public(int(i)).Address()
}

// Network is used by the Handler to send a DKG protocol packet over the network.
type Network interface {
	Send(net.Peer, *dkg_proto.Packet) error
}

type intArray []int

func (i intArray) String() string {
	var s bytes.Buffer
	s.WriteString("[")
	var sarr = make([]string, len(i))
	for idx, v := range i {
		sarr[idx] = strconv.Itoa(v)
	}
	s.WriteString(strings.Join(sarr, ","))
	s.WriteString("]")
	return s.String()
}
