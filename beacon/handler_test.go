package beacon

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	//"github.com/benbjohnson/clock"
	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/drand/net"
	"github.com/drand/drand/protobuf/drand"
	"github.com/drand/drand/test"
	"github.com/drand/kyber"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/util/random"
	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
)

// TODO make beacon tests not dependant on key.Scheme

// testBeaconServer implements a barebone service to be plugged in a net.DefaultService
type testBeaconServer struct {
	disable bool
	*net.EmptyServer
	h *Handler
}

func (t *testBeaconServer) PartialBeacon(c context.Context, in *drand.PartialBeaconPacket) (*drand.Empty, error) {
	if t.disable {
		return nil, errors.New("disabled server")
	}
	return t.h.ProcessPartialBeacon(c, in)
}

func (t *testBeaconServer) SyncChain(req *drand.SyncRequest, p drand.Protocol_SyncChainServer) error {
	if t.disable {
		return errors.New("disabled server")
	}
	return t.h.SyncChain(req, p)
}

func dkgShares(n, t int) ([]*key.Share, []kyber.Point) {
	var priPoly *share.PriPoly
	var pubPoly *share.PubPoly
	var err error
	for i := 0; i < n; i++ {
		pri := share.NewPriPoly(key.KeyGroup, t, key.KeyGroup.Scalar().Pick(random.New()), random.New())
		pub := pri.Commit(key.KeyGroup.Point().Base())
		if priPoly == nil {
			priPoly = pri
			pubPoly = pub
			continue
		}
		priPoly, err = priPoly.Add(pri)
		if err != nil {
			panic(err)
		}
		pubPoly, err = pubPoly.Add(pub)
		if err != nil {
			panic(err)
		}
	}
	shares := priPoly.Shares(n)
	secret, err := share.RecoverSecret(key.KeyGroup, shares, t, n)
	if err != nil {
		panic(err)
	}
	if !secret.Equal(priPoly.Secret()) {
		panic("secret not equal")
	}
	msg := []byte("Hello world")
	sigs := make([][]byte, n, n)
	_, commits := pubPoly.Info()
	dkgShares := make([]*key.Share, n, n)
	for i := 0; i < n; i++ {
		sigs[i], err = key.Scheme.Sign(shares[i], msg)
		if err != nil {
			panic(err)
		}
		dkgShares[i] = &key.Share{
			Share:   shares[i],
			Commits: commits,
		}
	}
	sig, err := key.Scheme.Recover(pubPoly, msg, sigs, t, n)
	if err != nil {
		panic(err)
	}
	if err := key.Scheme.VerifyRecovered(pubPoly.Commit(), msg, sig); err != nil {
		panic(err)
	}
	//fmt.Println(pubPoly.Commit())
	return dkgShares, commits
}

type node struct {
	index    int // group index
	private  *key.Pair
	shares   *key.Share
	callback func(*Beacon)
	handler  *Handler
	listener net.Listener
	clock    clock.FakeClock
	started  bool
	server   *testBeaconServer
}

type BeaconTest struct {
	paths   []string
	n       int
	thr     int
	genesis int64
	shares  []*key.Share
	period  time.Duration
	group   *key.Group
	privs   []*key.Pair
	dpublic kyber.Point
	nodes   map[int]*node
	time    clock.FakeClock
	prefix  string
}

func NewBeaconTest(n, thr int, period time.Duration, genesisTime int64) *BeaconTest {
	prefix, err := ioutil.TempDir(os.TempDir(), "beacon-test")
	checkErr(err)
	paths := createBoltStores(prefix, n)
	shares, commits := dkgShares(n, thr)
	privs, group := test.BatchIdentities(n)
	group.Threshold = thr
	group.Period = period
	group.GenesisTime = genesisTime
	group.PublicKey = &key.DistPublic{commits}

	bt := &BeaconTest{
		prefix:  prefix,
		n:       n,
		privs:   privs,
		thr:     thr,
		period:  period,
		paths:   paths,
		shares:  shares,
		group:   group,
		dpublic: group.PublicKey.PubPoly().Commit(),
		nodes:   make(map[int]*node),
		time:    clock.NewFakeClock(),
	}

	for i := 0; i < n; i++ {
		bt.CreateNode(i)
	}
	return bt
}

func (b *BeaconTest) CreateNode(i int) {
	findShare := func(target int) *key.Share {
		for _, s := range b.shares {
			if s.Share.I == target {
				return s
			}
		}
		panic("we should always get a share")
	}
	for _, p := range b.privs {
		idx, _ := b.group.Index(p.Public)
		if idx != i {
			continue
		}
		node := &node{}
		if n, ok := b.nodes[idx]; ok {
			node = n
		}
		node.index = idx
		node.private = p
		share := findShare(idx)
		node.shares = share
		store, err := NewBoltStore(b.paths[idx], nil)
		if err != nil {
			panic(err)
		}
		node.clock = clock.NewFakeClockAt(b.time.Now())
		conf := &Config{
			Group:   b.group,
			Private: p,
			Share:   share,
			Clock:   node.clock,
		}

		node.handler, err = NewHandler(net.NewGrpcClient(), store, conf, log.NewLogger(log.LogDebug))
		checkErr(err)
		if node.callback != nil {
			node.handler.callbacks.AddCallback(node.callback)
		}

		if node.handler.addr != node.private.Public.Address() {
			panic("Oh Oh")
		}

		currSig, err := key.Scheme.Sign(node.handler.conf.Share.PrivateShare(), []byte("hello"))
		checkErr(err)
		sigIndex, _ := key.Scheme.IndexOf(currSig)
		if sigIndex != idx {
			panic("invalid index")
		}
		b.nodes[idx] = node
		fmt.Printf("\n NODE index %d --> Listens on %s || Clock pointer %p\n", idx, p.Public.Address(), b.nodes[idx].handler.conf.Clock)
	}
	for i, n := range b.nodes {
		for j, n2 := range b.nodes {
			if i == j {
				continue
			}
			if n.index == n2.index {
				panic("invalid index setting")
			}
		}
	}
}

func (b *BeaconTest) ServeBeacon(i int) {
	j := b.searchNode(i)
	beaconServer := &testBeaconServer{h: b.nodes[j].handler}
	b.nodes[j].server = beaconServer
	b.nodes[j].listener = net.NewTCPGrpcListener(b.nodes[j].private.Public.Address(), beaconServer)
	fmt.Printf("\n || Serve Beacon for node %d - %p --> %s\n", j, b.nodes[j].handler, b.nodes[j].private.Public.Address())
	go b.nodes[j].listener.Start()
}

func (b *BeaconTest) StartBeacons(n int) {
	for i := 0; i < n; i++ {
		b.StartBeacon(i, false)
	}
	// give time for go routines to kick off
	time.Sleep(1000 * time.Millisecond)
}
func (b *BeaconTest) StartBeacon(i int, catchup bool) {
	j := b.searchNode(i)
	b.nodes[j].started = true
	if catchup {
		fmt.Printf("\t Start BEACON %s - node pointer %p\n", b.nodes[j].handler.addr, b.nodes[j].handler)
		go b.nodes[j].handler.Catchup()
	} else {
		go b.nodes[j].handler.Start()
	}
}

func (b *BeaconTest) searchNode(i int) int {
	for j, n := range b.nodes {
		if n.index == i {
			return j
		}
	}
	panic("no such index")
}
func (b *BeaconTest) MoveTime(t time.Duration) {
	for _, n := range b.nodes {
		before := n.clock.Now().Unix()
		n.handler.conf.Clock.(clock.FakeClock).Advance(t)
		fmt.Printf(" - %d increasing time of node %d - %s (pointer %p)- before: %d - current: %d - pointer clock %p\n", time.Now().Unix(), n.index, n.private.Public.Address(), n, before, n.clock.Now().Unix(), n.handler.conf.Clock)
	}
	b.time.Advance(t)
	// give each handlers time to perform their duty
	time.Sleep(time.Duration(b.n*100) * time.Millisecond)
	//time.Sleep(100 * time.Millisecond)
}

func (b *BeaconTest) StopBeacon(i int) {
	j := b.searchNode(i)
	if n, ok := b.nodes[j]; ok {
		if !n.started {
			return
		}
		n.listener.Stop()
		n.handler.Stop()
		n.started = false
	}
	delete(b.nodes, j)
}

func (b *BeaconTest) StopAll() {
	for _, n := range b.nodes {
		b.StopBeacon(n.index)
	}
}

func (b *BeaconTest) CleanUp() {
	deleteBoltStores(b.prefix)
	b.StopAll()
}

func (b *BeaconTest) DisableReception(count int) {
	for i := 0; i < count; i++ {
		b.nodes[i].server.disable = true
	}
}

func (b *BeaconTest) EnableReception(count int) {
	for i := 0; i < count; i++ {
		b.nodes[i].server.disable = false
	}
}

func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}

func createBoltStores(prefix string, n int) []string {
	paths := make([]string, n, n)
	for i := 0; i < n; i++ {
		paths[i] = path.Join(prefix, fmt.Sprintf("drand-%d", i))
		if err := os.MkdirAll(paths[i], 0755); err != nil {
			panic(err)
		}
	}
	return paths
}

func deleteBoltStores(prefix string) {
	os.RemoveAll(prefix)
}

func checkWait(counter *sync.WaitGroup) {
	var doneCh = make(chan bool, 1)
	go func() {
		counter.Wait()
		doneCh <- true
	}()
	select {
	case <-doneCh:
		break
	case <-time.After(1 * time.Second):
		fmt.Println(" _------------- OUTDATED ----------------")
		panic("outdated beacon time")
	}
}

func TestBeaconSync(t *testing.T) {
	n := 5
	thr := n/2 + 1
	period := 2 * time.Second

	var genesisOffset = 2 * time.Second
	var genesisTime int64 = clock.NewFakeClock().Now().Add(genesisOffset).Unix()
	bt := NewBeaconTest(n, thr, period, genesisTime)
	defer bt.CleanUp()
	var counter = &sync.WaitGroup{}
	myCallBack := func(i int) func(*Beacon) {
		return func(b *Beacon) {
			require.NoError(t, VerifyBeacon(bt.dpublic, b))
			fmt.Printf("\nROUND %d DONE for %s\n\n", b.Round, bt.nodes[bt.searchNode(i)].private.Public.Address())
			counter.Done()
		}
	}

	doRound := func(count int, move time.Duration) {
		counter.Add(count)
		bt.MoveTime(move)
		checkWait(counter)
	}

	for i := 0; i < n; i++ {
		bt.CallbackFor(i, myCallBack(i))
		bt.ServeBeacon(i)
	}
	bt.StartBeacons(n)

	// move clock to genesis time
	fmt.Printf("\n\n --- BEFORE GENESIS --- \n\n")
	doRound(n, genesisOffset)
	fmt.Printf("\n\n --- AFTER GENESIS --- \n\n")
	// do some rounds
	for i := 0; i < 2; i++ {
		doRound(n, period)
		fmt.Printf(" \n\n --- ROUND DONE %d \n\n", i)
	}

	// disable reception of all nodes but one
	online := 2
	bt.DisableReception(n - online)
	// check that at least one node got the beacon
	doRound(online, period)
	fmt.Printf("\n\n-- BEFORE ENABLING RECEPTION AGAIN -- \n\n")
	// enable reception again of all nodes
	bt.EnableReception(n - online)
	// we advance the clock, all "resucitated nodes" will transmit a wrong
	// beacon, but they will see the beacon they send is late w.r.t. the round
	// they should be, so they will sync with the "safe online" nodes. They
	// will get the latest beacon and then directly run the right round
	//bt.MoveTime(period
	// n for the new round
	// n - online for the previous round that the others catch up
	doRound(n+n-online, period)
}
func TestBeaconSimple(t *testing.T) {
	n := 3
	thr := n/2 + 1
	period := 2 * time.Second

	//var genesisTime int64 = clock.NewMock().Now().Unix() + 2
	var genesisTime int64 = clock.NewFakeClock().Now().Unix() + 2

	bt := NewBeaconTest(n, thr, period, genesisTime)
	defer bt.CleanUp()

	var counter = &sync.WaitGroup{}
	counter.Add(n)
	myCallBack := func(b *Beacon) {
		// verify partial sig
		require.NoError(t, VerifyBeacon(bt.dpublic, b))
		//msg := Message(b.PreviousSig, b.PreviousRound, b.Round)
		//err := key.Scheme.VerifyRecovered(bt.dpublic, msg, b.Signature)
		//require.NoError(t, err)
		counter.Done()
	}

	for i := 0; i < n; i++ {
		bt.CallbackFor(i, myCallBack)
		// first serve all beacons
		bt.ServeBeacon(i)
	}

	bt.StartBeacons(n)
	// move clock before genesis time
	bt.MoveTime(1 * time.Second)
	for i := 0; i < n; i++ {
		bt.nodes[i].handler.Lock()
		started := bt.nodes[i].handler.started
		bt.nodes[i].handler.Unlock()
		require.False(t, started, "handler %d has started?", i)
		//fmt.Printf(" + before genesis - node %d has clock time %d\n", bt.handlers[i].index, bt.handlers[i].conf.Clock.Now().Unix())
	}
	fmt.Println(" --------- moving to genesis ---------------")
	// move clock to genesis time
	bt.MoveTime(1 * time.Second)

	// check 1 period
	checkWait(counter)
	// check 2 period
	counter.Add(n)
	bt.MoveTime(period)
	checkWait(counter)
}

func TestBeaconThreshold(t *testing.T) {
	n := 3
	thr := n/2 + 1
	period := 2 * time.Second

	offsetGenesis := 2 * time.Second
	var genesisTime int64 = clock.NewFakeClock().Now().Add(offsetGenesis).Unix()

	bt := NewBeaconTest(n, thr, period, genesisTime)
	defer func() { go bt.CleanUp() }()
	var currentRound uint64 = 0
	var counter = &sync.WaitGroup{}
	myCallBack := func(i int) func(*Beacon) {
		return func(b *Beacon) {
			fmt.Printf(" - test: callback called for node %d - round %d\n", i, b.Round)
			// verify partial sig
			msg := Message(b.Round, b.PreviousSig)
			err := key.Scheme.VerifyRecovered(bt.dpublic, msg, b.Signature)
			require.NoError(t, err)
			// callbacks are called for syncing up as well so we only decrease
			// waitgroup when it's the current round
			if b.Round == currentRound {
				counter.Done()
			}
		}
	}

	makeRounds := func(r int, howMany int) {
		func() {
			for i := 0; i < r; i++ {
				currentRound++
				counter.Add(howMany)
				bt.MoveTime(period)
				checkWait(counter)
				time.Sleep(100 * time.Millisecond)
			}
		}()
	}
	nRounds := 1
	// open connections for all but one
	for i := 0; i < n-1; i++ {
		bt.CallbackFor(i, myCallBack(i))
		bt.ServeBeacon(i)
	}

	// start all but one
	bt.StartBeacons(n - 1)
	// move to genesis time and check they ran the round 1
	currentRound = 1
	counter.Add(n - 1)
	bt.MoveTime(offsetGenesis)
	checkWait(counter)

	// make a few rounds
	makeRounds(nRounds, n-1)

	// launch the last one
	bt.ServeBeacon(n - 1)
	bt.StartBeacon(n-1, true)
	fmt.Printf("\nLAST NODE LAUNCHED ! \n\n")
	// wait a bit for syncing to take place
	time.Sleep(100 * time.Millisecond)
	bt.CallbackFor(n-1, myCallBack(n-1))
	fmt.Printf("\n | MAKE NEW ROUNDS |\n\n")
	// and then run a few rounds
	makeRounds(nRounds, n)

	/*fmt.Printf("\n | STOP LAST NODE |\n\n")*/
	//// stop last one again - so it will force a sync not from genesis
	//bt.StopBeacon(n - 1)
	//// make a few round
	//makeRounds(nRounds, n-1)

	//fmt.Printf("\n | CREATE LAST NODE AGAIN | \n\n")
	//// start the node again
	//bt.CreateNode(n - 1)
	//bt.ServeBeacon(n - 1)
	//bt.StartBeacon(n-1, true)
	//bt.CallbackFor(n-1, myCallBack(n-1))
	//// let time for syncing
	/*time.Sleep(100 * time.Millisecond)*/
	fmt.Printf("\n | MOVE TIME WITH ALL NODES  | \n\n")
	// expect lastnode to have catch up
	makeRounds(nRounds, n)
}

func (b *BeaconTest) CallbackFor(i int, fn func(*Beacon)) {
	j := b.searchNode(i)
	b.nodes[j].handler.callbacks.AddCallback(fn)
}
