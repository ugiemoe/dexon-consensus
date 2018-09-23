// Copyright 2018 The dexon-consensus-core Authors
// This file is part of the dexon-consensus-core library.
//
// The dexon-consensus-core library is free software: you can redistribute it
// and/or modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.
//
// The dexon-consensus-core library is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser
// General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the dexon-consensus-core library. If not, see
// <http://www.gnu.org/licenses/>.

package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/dexon-foundation/dexon-consensus-core/common"
	"github.com/dexon-foundation/dexon-consensus-core/core/test"
	"github.com/dexon-foundation/dexon-consensus-core/core/types"
	"github.com/dexon-foundation/dexon-consensus-core/simulation/config"
)

type messageType string

const (
	shutdownAck    messageType = "shutdownAck"
	blockTimestamp messageType = "blockTimestamps"
)

// message is a struct for peer sending message to server.
type message struct {
	Type    messageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type timestampEvent string

const (
	blockSeen        timestampEvent = "blockSeen"
	timestampConfirm timestampEvent = "timestampConfirm"
	timestampAck     timestampEvent = "timestampAck"
)

// TimestampMessage is a struct for peer sending consensus timestamp information
// to server.
type timestampMessage struct {
	BlockHash common.Hash    `json:"hash"`
	Event     timestampEvent `json:"event"`
	Timestamp time.Time      `json:"timestamp"`
}

type infoStatus string

const (
	statusInit     infoStatus = "init"
	statusNormal   infoStatus = "normal"
	statusShutdown infoStatus = "shutdown"
)

// infoMessage is a struct used by peerServer's /info.
type infoMessage struct {
	Status infoStatus              `json:"status"`
	Peers  map[types.NodeID]string `json:"peers"`
}

// network implements core.Network interface and other methods for simulation
// based on test.TransportClient.
type network struct {
	cfg           config.Networking
	ctx           context.Context
	ctxCancel     context.CancelFunc
	trans         test.TransportClient
	fromTransport <-chan *test.TransportEnvelope
	toConsensus   chan interface{}
	toNode        chan interface{}
}

// newNetwork setup network stuffs for nodes, which provides an
// implementation of core.Network based on test.TransportClient.
func newNetwork(nID types.NodeID, cfg config.Networking) (n *network) {
	// Construct latency model.
	latency := &test.NormalLatencyModel{
		Mean:  cfg.Mean,
		Sigma: cfg.Sigma,
	}
	// Construct basic network instance.
	n = &network{
		cfg:         cfg,
		toNode:      make(chan interface{}, 1000),
		toConsensus: make(chan interface{}, 1000),
	}
	n.ctx, n.ctxCancel = context.WithCancel(context.Background())
	// Construct transport layer.
	switch cfg.Type {
	case config.NetworkTypeTCPLocal:
		n.trans = test.NewTCPTransportClient(
			nID, latency, &jsonMarshaller{}, true)
	case config.NetworkTypeTCP:
		n.trans = test.NewTCPTransportClient(
			nID, latency, &jsonMarshaller{}, false)
	case config.NetworkTypeFake:
		n.trans = test.NewFakeTransportClient(nID, latency)
	default:
		panic(fmt.Errorf("unknown network type: %v", cfg.Type))
	}
	return
}

// BroadcastVote implements core.Network interface.
func (n *network) BroadcastVote(vote *types.Vote) {
	if err := n.trans.Broadcast(vote); err != nil {
		panic(err)
	}
}

// BroadcastBlock implements core.Network interface.
func (n *network) BroadcastBlock(block *types.Block) {
	if err := n.trans.Broadcast(block); err != nil {
		panic(err)
	}
}

// BroadcastWitnessAck implements core.Network interface.
func (n *network) BroadcastWitnessAck(witnessAck *types.WitnessAck) {
	if err := n.trans.Broadcast(witnessAck); err != nil {
		panic(err)
	}
}

// broadcast message to all other nodes in the network.
func (n *network) broadcast(message interface{}) {
	if err := n.trans.Broadcast(message); err != nil {
		panic(err)
	}
}

// SendDKGPrivateShare implements core.Network interface.
func (n *network) SendDKGPrivateShare(
	recv types.NodeID, prvShare *types.DKGPrivateShare) {
	if err := n.trans.Send(recv, prvShare); err != nil {
		panic(err)
	}
}

// BroadcastDKGPrivateShare implements core.Network interface.
func (n *network) BroadcastDKGPrivateShare(
	prvShare *types.DKGPrivateShare) {
	if err := n.trans.Broadcast(prvShare); err != nil {
		panic(err)
	}
}

// ReceiveChan implements core.Network interface.
func (n *network) ReceiveChan() <-chan interface{} {
	return n.toConsensus
}

// receiveChanForNode returns a channel for nodes' specific
// messages.
func (n *network) receiveChanForNode() <-chan interface{} {
	return n.toNode
}

// setup transport layer.
func (n *network) setup(serverEndpoint interface{}) (err error) {
	// Join the p2p network.
	switch n.cfg.Type {
	case config.NetworkTypeTCP, config.NetworkTypeTCPLocal:
		addr := net.JoinHostPort(n.cfg.PeerServer, strconv.Itoa(peerPort))
		n.fromTransport, err = n.trans.Join(addr)
	case config.NetworkTypeFake:
		n.fromTransport, err = n.trans.Join(serverEndpoint)
	default:
		err = fmt.Errorf("unknown network type: %v", n.cfg.Type)
	}
	if err != nil {
		return
	}
	return
}

// run the main loop.
func (n *network) run() {
	// The dispatcher declararion:
	// to consensus or node, that's the question.
	disp := func(e *test.TransportEnvelope) {
		switch e.Msg.(type) {
		case *types.Block, *types.Vote, *types.WitnessAck,
			*types.DKGPrivateShare, *types.DKGPartialSignature:
			n.toConsensus <- e.Msg
		default:
			n.toNode <- e.Msg
		}
	}
MainLoop:
	for {
		select {
		case <-n.ctx.Done():
			break MainLoop
		default:
		}
		select {
		case <-n.ctx.Done():
			break MainLoop
		case e, ok := <-n.fromTransport:
			if !ok {
				break MainLoop
			}
			disp(e)
		}
	}
}

// Close stop the network.
func (n *network) Close() (err error) {
	n.ctxCancel()
	close(n.toConsensus)
	n.toConsensus = nil
	close(n.toNode)
	n.toNode = nil
	if err = n.trans.Close(); err != nil {
		return
	}
	return
}

// report exports 'Report' method of test.TransportClient.
func (n *network) report(msg interface{}) error {
	return n.trans.Report(msg)
}

// peers exports 'Peers' method of test.Transport.
func (n *network) peers() map[types.NodeID]struct{} {
	return n.trans.Peers()
}
