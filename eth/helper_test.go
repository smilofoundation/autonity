// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// This file contains some shares testing functionality, common to  multiple
// different files and modules being tested.

package eth

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"testing"

	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/consensus/ethash"
	"github.com/clearmatics/autonity/core"
	"github.com/clearmatics/autonity/core/forkid"
	"github.com/clearmatics/autonity/core/rawdb"
	"github.com/clearmatics/autonity/core/types"
	"github.com/clearmatics/autonity/core/vm"
	"github.com/clearmatics/autonity/crypto"
	"github.com/clearmatics/autonity/eth/downloader"
	"github.com/clearmatics/autonity/ethdb"
	"github.com/clearmatics/autonity/event"
	"github.com/clearmatics/autonity/log"
	"github.com/clearmatics/autonity/p2p"
	"github.com/clearmatics/autonity/p2p/enode"
	"github.com/clearmatics/autonity/params"
)

var (
	testBankKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testBank       = crypto.PubkeyToAddress(testBankKey.PublicKey)
)

// newTestProtocolManager creates a new protocol manager for testing purposes,
// with the given number of blocks already known, and potential notification
// channels for different events.
func newTestProtocolManager(mode downloader.SyncMode, blocks int, generator func(int, *core.BlockGen), newtx chan<- []*types.Transaction, peers []string) (*ProtocolManager, ethdb.Database, error) {
	var (
		evmux  = new(event.TypeMux)
		engine = ethash.NewFaker()
		db     = rawdb.NewMemoryDatabase()
		gspec  = &core.Genesis{
			Config: params.TestChainConfig,
			Alloc:  core.GenesisAlloc{testBank: {Balance: big.NewInt(1000000)}},
		}
	)

	setupAutonityContract(gspec, peers)

	genesis := gspec.MustCommit(db)
	blockchain, err := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{}, nil, core.NewTxSenderCacher(), nil)
	if err != nil {
		panic(err)
	}

	chain, _ := core.GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, blocks, generator)
	if _, err := blockchain.InsertChain(chain); err != nil {
		panic(err)
	}
	pm, err := NewProtocolManager(gspec.Config, nil, mode, DefaultConfig.NetworkId, evmux, &testTxPool{added: newtx, pool: map[common.Hash]*types.Transaction{}}, engine, blockchain, db, 1, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	pm.Start(1000)
	return pm, db, nil
}

// helper that add some required fields in the genesis to bootstrap an Autonity Protocol Manager
func setupAutonityContract(gspec *core.Genesis, peers []string) *core.Genesis {
	gspec.Config.AutonityContractConfig = &params.AutonityContractGenesis{}

	for i := range peers {
		gspec.Config.AutonityContractConfig.Users = append(
			gspec.Config.AutonityContractConfig.Users,
			params.User{
				Enode: peers[i],
				Type:  params.UserValidator,
				Stake: 100,
			},
		)
	}
	gspec.Difficulty = big.NewInt(1)
	err := gspec.Config.AutonityContractConfig.Prepare()
	if err != nil {
		panic(err)
	}
	return gspec
}

// newTestProtocolManagerMust creates a new protocol manager for testing purposes,
// with the given number of blocks already known, and potential notification
// channels for different events. In case of an error, the constructor force-
// fails the test.
func newTestProtocolManagerMust(t *testing.T, mode downloader.SyncMode, blocks int, generator func(int, *core.BlockGen), newtx chan<- []*types.Transaction, peers []string) (*ProtocolManager, ethdb.Database) {
	pm, db, err := newTestProtocolManager(mode, blocks, generator, newtx, peers)
	if err != nil {
		t.Fatalf("Failed to create protocol manager: %v", err)
	}
	return pm, db
}

// testTxPool is a fake, helper transaction pool for testing purposes
type testTxPool struct {
	txFeed event.Feed
	pool   map[common.Hash]*types.Transaction // Hash map of collected transactions
	added  chan<- []*types.Transaction        // Notification channel for new transactions

	lock sync.RWMutex // Protects the transaction pool
}

// Has returns an indicator whether txpool has a transaction
// cached with the given hash.
func (p *testTxPool) Has(hash common.Hash) bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.pool[hash] != nil
}

// Get retrieves the transaction from local txpool with given
// tx hash.
func (p *testTxPool) Get(hash common.Hash) *types.Transaction {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.pool[hash]
}

// AddRemotes appends a batch of transactions to the pool, and notifies any
// listeners if the addition channel is non nil
func (p *testTxPool) AddRemotes(txs []*types.Transaction) []error {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, tx := range txs {
		p.pool[tx.Hash()] = tx
	}
	if p.added != nil {
		p.added <- txs
	}
	p.txFeed.Send(core.NewTxsEvent{Txs: txs})
	return make([]error, len(txs))
}

// Pending returns all the transactions known to the pool
func (p *testTxPool) Pending() (map[common.Address]types.Transactions, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	batches := make(map[common.Address]types.Transactions)
	for _, tx := range p.pool {
		from, _ := types.Sender(types.HomesteadSigner{}, tx)
		batches[from] = append(batches[from], tx)
	}
	for _, batch := range batches {
		sort.Sort(types.TxByNonce(batch))
	}
	return batches, nil
}

func (p *testTxPool) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return p.txFeed.Subscribe(ch)
}

// newTestTransaction create a new dummy transaction.
func newTestTransaction(from *ecdsa.PrivateKey, nonce uint64, datasize int) *types.Transaction {
	tx := types.NewTransaction(nonce, common.Address{}, big.NewInt(0), 100000, big.NewInt(0), make([]byte, datasize))
	tx, _ = types.SignTx(tx, types.HomesteadSigner{}, from)
	return tx
}

// testPeer is a simulated peer to allow testing direct network calls.
type testPeer struct {
	net p2p.MsgReadWriter // Network layer reader/writer to simulate remote messaging
	app *p2p.MsgPipeRW    // Application layer reader/writer to simulate the local side
	*peer
}

// newTestPeer creates a new peer registered at the given protocol manager.
func newTestPeer(p2pPeer *p2p.Peer, version int, pm *ProtocolManager, shake bool) (*testPeer, <-chan error) {
	// Create a message pipe to communicate through
	app, net := p2p.MsgPipe()

	// Start the peer on a new thread
	peer := pm.newPeer(version, p2pPeer, net, pm.txpool.Get)
	errc := make(chan error, 1)
	go func() { errc <- pm.runPeer(peer) }()
	tp := &testPeer{app: app, net: net, peer: peer}

	// Execute any implicitly requested handshakes and return
	if shake {
		var (
			genesis = pm.blockchain.Genesis()
			head    = pm.blockchain.CurrentHeader()
			td      = pm.blockchain.GetTd(head.Hash(), head.Number.Uint64())
		)
		forkID := forkid.NewID(pm.blockchain.Config(), pm.blockchain.Genesis().Hash(), pm.blockchain.CurrentHeader().Number.Uint64())
		tp.handshake(nil, td, head.Hash(), genesis.Hash(), forkID, forkid.NewFilter(pm.blockchain))
	}

	return tp, errc
}

func newTestP2PPeer(name string) *p2p.Peer {
	// Generate a random id and create the peer
	var id enode.ID
	rand.Read(id[:])
	peer, err := p2p.NewTestPeer(name, []p2p.Cap{})
	if err != nil {
		log.Error("newTestP2PPeer err", "error", err)
	}
	return peer
}

// handshake simulates a trivial handshake that expects the same state from the
// remote side as we are simulating locally.
func (p *testPeer) handshake(t *testing.T, td *big.Int, head common.Hash, genesis common.Hash, forkID forkid.ID, forkFilter forkid.Filter) {
	var msg interface{}
	switch {
	case p.version == eth63:
		msg = &statusData63{
			ProtocolVersion: uint32(p.version),
			NetworkId:       DefaultConfig.NetworkId,
			TD:              td,
			CurrentBlock:    head,
			GenesisBlock:    genesis,
		}
	case p.version >= eth64:
		msg = &statusData{
			ProtocolVersion: uint32(p.version),
			NetworkID:       DefaultConfig.NetworkId,
			TD:              td,
			Head:            head,
			Genesis:         genesis,
			ForkID:          forkID,
		}
	default:
		panic(fmt.Sprintf("unsupported eth protocol version: %d", p.version))
	}
	if err := p2p.ExpectMsg(p.app, StatusMsg, msg); err != nil {
		t.Fatalf("status recv: %v", err)
	}
	if err := p2p.Send(p.app, StatusMsg, msg); err != nil {
		t.Fatalf("status send: %v", err)
	}
}

// close terminates the local side of the peer, notifying the remote protocol
// manager of termination.
func (p *testPeer) close() {
	p.app.Close()
}
