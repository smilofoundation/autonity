// Copyright 2017 The go-ethereum Authors
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

package core

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/consensus/tendermint/algorithm"
	"github.com/clearmatics/autonity/consensus/tendermint/crypto"
	"github.com/clearmatics/autonity/consensus/tendermint/events"
	"github.com/clearmatics/autonity/contracts/autonity"
	autonitycrypto "github.com/clearmatics/autonity/crypto"
	"github.com/clearmatics/autonity/rlp"
	"github.com/davecgh/go-spew/spew"
)

var errStopped error = errors.New("stopped")

// Start implements core.Tendermint.Start
func (c *core) Start(ctx context.Context, contract *autonity.Contract) {
	println("starting")
	atomic.StoreInt32(&c.stopped, 0)
	// Set the autonity contract
	c.autonityContract = contract
	ctx, c.cancel = context.WithCancel(ctx)

	// Subscribe
	c.eventsSub = c.backend.Subscribe(events.MessageEvent{}, &algorithm.Timeout{}, events.CommitEvent{})
	c.syncEventSub = c.backend.Subscribe(events.SyncEvent{})
	c.newUnminedBlockEventSub = c.backend.Subscribe(events.NewUnminedBlockEvent{})

	c.wg = &sync.WaitGroup{}
	// We need a separate go routine to keep c.latestPendingUnminedBlock up to date
	c.wg.Add(1)
	go c.handleNewUnminedBlockEvent(ctx)

	// Tendermint Finite State Machine discrete event loop
	c.wg.Add(1)
	go c.mainEventLoop(ctx)

	go c.backend.HandleUnhandledMsgs(ctx)
}

// stop implements core.Engine.stop
func (c *core) Stop() {
	println(addr(c.address), c.height, "stopping")
	c.valueSet.L.Lock()
	atomic.StoreInt32(&c.stopped, 1)
	c.valueSet.Signal()
	c.valueSet.L.Unlock()
	c.logger.Info("stopping tendermint.core", "addr", addr(c.address))

	c.cancel()

	// Unsubscribe
	c.eventsSub.Unsubscribe()
	c.syncEventSub.Unsubscribe()

	println(addr(c.address), c.height, "almost stopped")
	// Ensure all event handling go routines exit
	c.wg.Wait()
}

func (c *core) handleNewUnminedBlockEvent(ctx context.Context) {
	defer c.wg.Done()
eventLoop:
	for {
		select {
		case e, ok := <-c.newUnminedBlockEventSub.Chan():
			if !ok {
				break eventLoop
			}
			block := e.Data.(events.NewUnminedBlockEvent).NewUnminedBlock
			c.SetValue(&block)
		case <-ctx.Done():
			c.logger.Info("handleNewUnminedBlockEvent is stopped", "event", ctx.Err())
			break eventLoop
		}
	}
}

func (c *core) newHeight(ctx context.Context, height uint64) bool {
	newHeight := new(big.Int).SetUint64(height)
	// set the new height
	c.setHeight(newHeight)
	c.currentBlock = c.AwaitValue(newHeight)
	// Check for stopped
	if atomic.LoadInt32(&c.stopped) == 1 {
		return true
	}
	prevBlock, _ := c.backend.LastCommittedProposal()

	c.lastHeader = prevBlock.Header()
	committeeSet := c.createCommittee(prevBlock)
	c.setCommitteeSet(committeeSet)

	// Update internals of oracle
	c.ora.lastHeader = c.lastHeader
	c.ora.committeeSet = committeeSet

	// Handle messages for the new height
	r := c.algo.StartRound(newHeight.Uint64(), 0, algorithm.ValueID(c.currentBlock.Hash()))

	// If we are making a proposal, we need to ensure that we add the proposal
	// block to the msg store, so that it can be picked up in buildMessage.
	if r.Broadcast != nil {
		println(addr(c.address), "adding value", height, c.currentBlock.Hash().String())
		c.msgCache.addValue(c.currentBlock.Hash(), c.currentBlock)
	}

	// Note that we don't risk enterning an infinite loop here since
	// start round can only return results with brodcasts or schedules.
	// TODO actually don't return result from Start round.
	stopped := c.handleResult(ctx, r)
	if stopped {
		return true
	}
	for _, msg := range c.msgCache.heightMessages(newHeight.Uint64()) {
		cm := c.msgCache.consensusMsgs[msg.Hash]
		go func(m *Message, cm *algorithm.ConsensusMessage) {
			err := c.handleCurrentHeightMessage(m, cm)
			c.logger.Error("failed to handle current height message", "message", m.String, "err", err)
		}(msg, cm)
	}
	return false
}

func (c *core) handleResult(ctx context.Context, r *algorithm.Result) bool {

	switch {
	case r == nil:
		return false
	case r.StartRound != nil:
		sr := r.StartRound
		if sr.Round == 0 && sr.Decision == nil {
			panic("round changes of 0 must be accompanied with a decision")
		}
		if sr.Decision != nil {
			// A decision has been reached
			println(addr(c.address), "decided on block", sr.Decision.Height,
				common.Hash(sr.Decision.Value).String())

			// This will ultimately lead to a commit event, which we will pick
			// up on but we will ignore it because instead we will wait here to
			// select the next value that matches this height.
			_, err := c.Commit(sr.Decision)
			if err != nil {
				panic(fmt.Sprintf("%s Failed to commit sr.Decision: %s err: %v", algorithm.NodeID(c.address).String(), spew.Sdump(sr.Decision), err))
			}
			stopped := c.newHeight(ctx, sr.Height)
			if stopped {
				return true
			}

		} else {
			// I don't think we need this switching
			// switch {
			// case currBlockNum > sr.Height:
			// 	panic(fmt.Sprintf("current block number %d cannot be greater than height %d", currBlockNum, sr.Height))
			// case currBlockNum < sr.Height:
			// 	c.currentBlock = c.AwaitValue(new(big.Int).SetUint64(sr.Height))
			// }

			// sanity check
			currBlockNum := c.currentBlock.Number().Uint64()
			if currBlockNum != sr.Height {
				panic(fmt.Sprintf("current block number %d out of sync with  height %d", currBlockNum, sr.Height))
			}

			r := c.algo.StartRound(sr.Height, sr.Round, algorithm.ValueID(c.currentBlock.Hash()))
			// Note that we don't risk enterning an infinite loop here since
			// start round can only return results with brodcasts or schedules.
			// TODO actually don't return result from Start round.
			stopped := c.handleResult(ctx, r)
			if stopped {
				return true
			}
		}
	case r.Broadcast != nil:
		println(addr(c.address), c.height.String(), r.Broadcast.String(), "sending")
		// Broadcasting ends with the message reaching us eventually

		// We must build message here since buildMessage relies on accessing
		// the msg store, and since the message stroe is not syncronised we
		// need to do it from the handler routine.
		msg := c.buildMessage(r.Broadcast)

		go c.broadcast(ctx, msg)
	case r.Schedule != nil:
		time.AfterFunc(time.Duration(r.Schedule.Delay)*time.Second, func() {
			c.backend.Post(r.Schedule)
		})

	}
	return false
}

func (c *core) mainEventLoop(ctx context.Context) {
	defer c.wg.Done()
	// Start a new round from last height + 1
	c.algo = algorithm.New(algorithm.NodeID(c.address), c.ora)
	lastBlockMined, _ := c.backend.LastCommittedProposal()
	stopped := c.newHeight(ctx, lastBlockMined.NumberU64()+1)
	if stopped {
		return
	}
	c.wg.Add(1)
	go c.syncLoop(ctx)

eventLoop:
	for {
		select {
		case ev, ok := <-c.eventsSub.Chan():
			if !ok {
				break eventLoop
			}
			// A real ev arrived, process interesting content
			switch e := ev.Data.(type) {
			case events.MessageEvent:
				if len(e.Payload) == 0 {
					c.logger.Error("core.mainEventLoop Get message(MessageEvent) empty payload")
				}

				if err := c.handleMsg(ctx, e.Payload); err != nil {
					if err == errStopped {
						return
					}
					c.logger.Debug("core.mainEventLoop Get message(MessageEvent) payload failed", "err", err)
					continue
				}
				c.backend.Gossip(ctx, c.committeeSet().Committee(), e.Payload)
			case *algorithm.ConsensusMessage:
				println(addr(c.address), e.String(), "message from self")
				// This is a message we sent ourselves we do not need to broadcast it
				if c.Height().Uint64() == e.Height {
					r := c.algo.ReceiveMessage(e)
					stopped := c.handleResult(ctx, r)
					if stopped {
						return
					}
				}
			case *algorithm.Timeout:
				var r *algorithm.Result
				switch e.TimeoutType {
				case algorithm.Propose:
					println(addr(c.address), "on timeout propose", e.Height, "round", e.Round)
					r = c.algo.OnTimeoutPropose(e.Height, e.Round)
				case algorithm.Prevote:
					println(addr(c.address), "on timeout prevote", e.Height, "round", e.Round)
					r = c.algo.OnTimeoutPrevote(e.Height, e.Round)
				case algorithm.Precommit:
					println(addr(c.address), "on timeout precommit", e.Height, "round", e.Round)
					r = c.algo.OnTimeoutPrecommit(e.Height, e.Round)
				}
				if r != nil && r.Broadcast != nil {
					println("nonnil timeout")
				}
				stopped := c.handleResult(ctx, r)
				if stopped {
					return
				}
			case events.CommitEvent:
				println(addr(c.address), "commit event")
				c.logger.Debug("Received a final committed proposal")
				lastBlock, _ := c.backend.LastCommittedProposal()
				height := new(big.Int).Add(lastBlock.Number(), common.Big1)
				if height.Cmp(c.Height()) == 0 {
					println(addr(c.address), "Discarding event as core is at the same height", "height", c.Height())
					c.logger.Debug("Discarding event as core is at the same height", "height", c.Height())
				} else {
					println(addr(c.address), "Received proposal is ahead", "height", c.Height().String(), "block_height", height.String())
					c.logger.Debug("Received proposal is ahead", "height", c.Height(), "block_height", height)
					stopped := c.newHeight(ctx, height.Uint64())
					if stopped {
						return
					}
				}
			}
		case <-ctx.Done():
			c.logger.Info("mainEventLoop is stopped", "event", ctx.Err())
			break eventLoop
		}
	}

}

func (c *core) syncLoop(ctx context.Context) {
	defer c.wg.Done()
	/*
		this method is responsible for asking the network to send us the current consensus state
		and to process sync queries events.
	*/
	timer := time.NewTimer(20 * time.Second)

	height := c.Height()

	// Ask for sync when the engine starts
	c.backend.AskSync(c.lastHeader)

eventLoop:
	for {
		select {
		case <-timer.C:
			currentHeight := c.Height()

			// we only ask for sync if the current view stayed the same for the past 10 seconds
			if currentHeight.Cmp(height) == 0 {
				c.backend.AskSync(c.lastHeader)
			}
			height = currentHeight
			timer = time.NewTimer(20 * time.Second)

		case ev, ok := <-c.syncEventSub.Chan():
			if !ok {
				break eventLoop
			}
			event := ev.Data.(events.SyncEvent)
			c.logger.Info("Processing sync message", "from", event.Addr)
			c.backend.SyncPeer(event.Addr)
		case <-ctx.Done():
			c.logger.Info("syncLoop is stopped", "event", ctx.Err())
			break eventLoop
		}
	}

}

func (c *core) handleMsg(ctx context.Context, payload []byte) error {

	/*
		Basic validity checks
	*/

	m := new(Message)

	// Set the hash on the message so that it can be used for indexing.
	m.Hash = common.BytesToHash(autonitycrypto.Keccak256(payload))

	// Check we haven't already processed this message
	if c.msgCache.Message(m.Hash) != nil {
		// Message was already processed
		return nil
	}

	// Decode message
	err := rlp.DecodeBytes(payload, m)
	if err != nil {
		return err
	}

	var proposal Proposal
	var preVote Vote
	var preCommit Vote
	var conMsg *algorithm.ConsensusMessage
	switch m.Code {
	case msgProposal:
		err := m.Decode(&proposal)
		if err != nil {
			return errFailedDecodeProposal
		}

		valueHash := proposal.ProposalBlock.Hash()
		conMsg = &algorithm.ConsensusMessage{
			MsgType:    algorithm.Step(m.Code),
			Height:     proposal.Height.Uint64(),
			Round:      proposal.Round,
			Value:      algorithm.ValueID(valueHash),
			ValidRound: proposal.ValidRound,
		}

		err = c.msgCache.addMessage(m, conMsg)
		if err != nil {
			// could be multiple proposal messages from the same proposer
			return err
		}
		c.msgCache.addValue(valueHash, proposal.ProposalBlock)

	case msgPrevote:
		err := m.Decode(&preVote)
		if err != nil {
			return errFailedDecodePrevote
		}
		conMsg = &algorithm.ConsensusMessage{
			MsgType: algorithm.Step(m.Code),
			Height:  preVote.Height.Uint64(),
			Round:   preVote.Round,
			Value:   algorithm.ValueID(preVote.ProposedBlockHash),
		}

		err = c.msgCache.addMessage(m, conMsg)
		if err != nil {
			// could be multiple precommits from same validator
			return err
		}
	case msgPrecommit:
		err := m.Decode(&preCommit)
		if err != nil {
			return errFailedDecodePrecommit
		}
		// Check the committed seal matches the block hash if its a precommit.
		// If not we ignore the message.
		//
		// Note this method does not make use of any blockchain state, so it is
		// safe to call it now. In fact it only uses the logger of c so I think
		// it could easily be detached from c.
		err = c.verifyCommittedSeal(m.Address, append([]byte(nil), m.CommittedSeal...), preCommit.ProposedBlockHash, preCommit.Round, preCommit.Height)
		if err != nil {
			return err
		}
		conMsg = &algorithm.ConsensusMessage{
			MsgType: algorithm.Step(m.Code),
			Height:  preCommit.Height.Uint64(),
			Round:   preCommit.Round,
			Value:   algorithm.ValueID(preCommit.ProposedBlockHash),
		}

		err = c.msgCache.addMessage(m, conMsg)
		if err != nil {
			// could be multiple precommits from same validator
			return err
		}
	default:
		return fmt.Errorf("unrecognised consensus message code %q", m.Code)
	}

	// If this message is for a future height then we cannot validate it
	// because we lack the relevant header, we will process it when we reach
	// that height. If it is for a previous height then we are not intersted in
	// it. But it has been added to the msg cache in case other peers would
	// like to sync it.
	if conMsg.Height != c.Height().Uint64() {
		// Nothing to do here
		return nil
	}

	return c.handleCurrentHeightMessage(m, conMsg)

}

func (c *core) handleCurrentHeightMessage(m *Message, cm *algorithm.ConsensusMessage) error {
	println(addr(c.address), c.height.String(), cm.String(), "received")
	/*
		Domain specific validity checks, now we know that we are at the same
		height as this message we can rely on lastHeader.
	*/

	// Check that the message came from a committee member, if not we ignore it.
	if c.lastHeader.CommitteeMember(m.Address) == nil {
		// TODO turn this into an error type that can be checked for at a
		// higher level to close the connection to this peer.
		return fmt.Errorf("received message from non committee member: %v", m)
	}

	payload, err := m.PayloadNoSig()
	if err != nil {
		return err
	}

	// Again we ignore messges with invalid signatures, they cannot be trusted.
	// TODO make crypto.CheckValidatorSignature accept Message so that it can
	// handle generating the payload and checking it with the sig and address.
	address, err := crypto.CheckValidatorSignature(c.lastHeader, payload, m.Signature)
	if err != nil {
		return err
	}

	if address != m.Address {
		// TODO why is Address even a field of Message when the address can be derived?
		return fmt.Errorf("address in message %q and address derived from signature %q don't match", m.Address, address)
	}

	switch m.Code {
	case msgProposal:
		// We ignore proposals from non proposers
		if !c.isProposerMsg(cm.Round, m.Address) {
			c.logger.Warn("Ignore proposal messages from non-proposer")
			return errNotFromProposer

			// TODO verify proposal here.
			//
			// If we are introducing time into the mix then what we are saying
			// is that we don't expect different participants' clocks to drift
			// out of sync more than some delta. And if they do then we don't
			// expect consensus to work.
			//
			// So in the case that clocks drift too far out of sync and say a
			// node considers a proposal invalid that 2f+1 other nodes
			// precommit for that node becomes stuck and can only continue in
			// consensus by re-syncing the blocks.
			//
			// So in verifying the proposal wrt time we should verify once
			// within reasonable clock sync bounds and then set the validity
			// based on that and never re-process the message again.

		}
		// Proposals values are allowed to be invalid.
		if _, err := c.backend.VerifyProposal(*c.msgCache.value(common.Hash(cm.Value))); err == nil {
			println(addr(c.address), "valid", cm.Value.String())
			c.msgCache.setValid(common.Hash(cm.Value))
		}
	default:
		// All other messages that have reached this point are valid, but we are not marking the vlaue valid here, we are marking the message valid.
		c.msgCache.setValid(m.Hash)
	}

	r := c.algo.ReceiveMessage(cm)
	stopped := c.handleResult(context.Background(), r)
	if stopped {
		return errStopped
	}
	return nil
}
