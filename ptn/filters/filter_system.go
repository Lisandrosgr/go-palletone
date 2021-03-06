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

// Package filters implements an ethereum filtering system for block,
// transactions and log events.
package filters

import (
	//"context"
	"errors"
	//"fmt"
	"sync"
	"time"

	ethereum "github.com/palletone/go-palletone"
	"github.com/palletone/go-palletone/common"
	"github.com/palletone/go-palletone/common/event"
	"github.com/palletone/go-palletone/common/rpc"
	"github.com/palletone/go-palletone/dag/modules"
)

// Type determines the kind of filter and is used to put the filter in to
// the correct bucket when added.
type Type byte

const (
	// UnknownSubscription indicates an unknown subscription type
	UnknownSubscription Type = iota
	// LogsSubscription queries for new or removed (chain reorg) logs
	LogsSubscription
	// PendingLogsSubscription queries for logs in pending blocks
	PendingLogsSubscription
	// MinedAndPendingLogsSubscription queries for logs in mined and pending blocks.
	MinedAndPendingLogsSubscription
	// PendingTransactionsSubscription queries tx hashes for pending
	// transactions entering the pending state
	PendingTransactionsSubscription
	// BlocksSubscription queries hashes for blocks that are imported
	BlocksSubscription
	// LastSubscription keeps track of the last index
	LastIndexSubscription
)

const (

	// txChanSize is the size of channel listening to TxPreEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096
	// rmLogsChanSize is the size of channel listening to RemovedLogsEvent.
	rmLogsChanSize = 10
	// logsChanSize is the size of channel listening to LogsEvent.
	logsChanSize = 10
	// chainEvChanSize is the size of channel listening to ChainEvent.
	chainEvChanSize = 10
)

var (
	ErrInvalidSubscriptionID = errors.New("invalid id")
)

type subscription struct {
	id        rpc.ID
	typ       Type
	created   time.Time
	logsCrit  ethereum.FilterQuery
	hashes    chan common.Hash
	headers   chan *modules.Header
	installed chan struct{} // closed when the filter is installed
	err       chan error    // closed when the filter is uninstalled
}

// EventSystem creates subscriptions, processes events and broadcasts them to the
// subscription which match the subscription criteria.
type EventSystem struct {
	mux       *event.TypeMux
	backend   Backend
	lightMode bool
	lastHead  *modules.Header
	install   chan *subscription // install filter for event notification
	uninstall chan *subscription // remove filter for event notification
}

// NewEventSystem creates a new manager that listens for event on the given mux,
// parses and filters them. It uses the all map to retrieve filter changes. The
// work loop holds its own index that is used to forward events to filters.
//
// The returned manager has a loop that needs to be stopped with the Stop function
// or by stopping the given mux.
func NewEventSystem(mux *event.TypeMux, backend Backend, lightMode bool) *EventSystem {
	m := &EventSystem{
		mux:       mux,
		backend:   backend,
		lightMode: lightMode,
		install:   make(chan *subscription),
		uninstall: make(chan *subscription),
	}

	//go m.eventLoop()//would recover

	return m
}

// Subscription is created when the client registers itself for a particular event.
type Subscription struct {
	ID        rpc.ID
	f         *subscription
	es        *EventSystem
	unsubOnce sync.Once
}

// Err returns a channel that is closed when unsubscribed.
func (sub *Subscription) Err() <-chan error {
	return sub.f.err
}

// Unsubscribe uninstalls the subscription from the event broadcast loop.
func (sub *Subscription) Unsubscribe() {
	sub.unsubOnce.Do(func() {
	uninstallLoop:
		for {
			// write uninstall request and consume logs/hashes. This prevents
			// the eventLoop broadcast method to deadlock when writing to the
			// filter event channel while the subscription loop is waiting for
			// this method to return (and thus not reading these events).
			select {
			case sub.es.uninstall <- sub.f:
				break uninstallLoop
			//case <-sub.f.logs:
			case <-sub.f.hashes:
			case <-sub.f.headers:
			}
		}

		// wait for filter to be uninstalled in work loop before returning
		// this ensures that the manager won't use the event channel which
		// will probably be closed by the client asap after this method returns.
		<-sub.Err()
	})
}

// subscribe installs the subscription in the event broadcast loop.
func (es *EventSystem) subscribe(sub *subscription) *Subscription {
	es.install <- sub
	<-sub.installed
	return &Subscription{ID: sub.id, f: sub, es: es}
}

// SubscribeNewHeads creates a subscription that writes the header of a block that is
// imported in the chain.
func (es *EventSystem) SubscribeNewHeads(headers chan *modules.Header) *Subscription {
	sub := &subscription{
		id:        rpc.NewID(),
		typ:       BlocksSubscription,
		created:   time.Now(),
		hashes:    make(chan common.Hash),
		headers:   headers,
		installed: make(chan struct{}),
		err:       make(chan error),
	}
	return es.subscribe(sub)
}

// SubscribePendingTxEvents creates a subscription that writes transaction hashes for
// transactions that enter the transaction pool.
func (es *EventSystem) SubscribePendingTxEvents(hashes chan common.Hash) *Subscription {
	sub := &subscription{
		id:        rpc.NewID(),
		typ:       PendingTransactionsSubscription,
		created:   time.Now(),
		hashes:    hashes,
		headers:   make(chan *modules.Header),
		installed: make(chan struct{}),
		err:       make(chan error),
	}
	return es.subscribe(sub)
}

type filterIndex map[Type]map[rpc.ID]*subscription

// broadcast event to filters that match criteria.
func (es *EventSystem) broadcast(filters filterIndex, ev interface{}) {
	if ev == nil {
		return
	}

	switch e := ev.(type) {
	case modules.TxPreEvent:
		for _, f := range filters[PendingTransactionsSubscription] {
			f.hashes <- e.Tx.TxHash
		}

	}
}

func (es *EventSystem) lightFilterNewHead(newHeader *modules.Header, callBack func(*modules.Header, bool)) {
	/*
		oldh := es.lastHead
		es.lastHead = newHeader
		if oldh == nil {
			return
		}
		newh := newHeader
		// find common ancestor, create list of rolled back and new block hashes
		var oldHeaders, newHeaders []*modules.Header
		for oldh.Hash() != newh.Hash() {
			if oldh.Number.Uint64() >= newh.Number.Uint64() {
				oldHeaders = append(oldHeaders, oldh)
				oldh = coredata.GetHeader(es.backend.ChainDb(), oldh.ParentHash, oldh.Number.Uint64()-1)
			}
			if oldh.Number.Uint64() < newh.Number.Uint64() {
				newHeaders = append(newHeaders, newh)
				newh = coredata.GetHeader(es.backend.ChainDb(), newh.ParentHash, newh.Number.Uint64()-1)
				if newh == nil {
					// happens when CHT syncing, nothing to do
					newh = oldh
				}
			}
		}
		// roll back old blocks
		for _, h := range oldHeaders {
			callBack(h, true)
		}
		// check new blocks (array is in reverse order)
		for i := len(newHeaders) - 1; i >= 0; i-- {
			callBack(newHeaders[i], false)
		}
	*/
}

// eventLoop (un)installs filters and processes mux events.
func (es *EventSystem) eventLoop() {

	var (
		index = make(filterIndex)
		//sub   = es.mux.Subscribe(coredata.PendingLogsEvent{})
		// Subscribe TxPreEvent form txpool
		txCh  = make(chan modules.TxPreEvent, txChanSize)
		txSub = es.backend.SubscribeTxPreEvent(txCh)
		// Subscribe RemovedLogsEvent
		//		rmLogsCh  = make(chan core.RemovedLogsEvent, rmLogsChanSize)
		//		rmLogsSub = es.backend.SubscribeRemovedLogsEvent(rmLogsCh)
		//		logsCh  = make(chan []*types.Log, logsChanSize)
		//		logsSub = es.backend.SubscribeLogsEvent(logsCh)
		//		// Subscribe ChainEvent
		//		chainEvCh  = make(chan core.ChainEvent, chainEvChanSize)
		//		chainEvSub = es.backend.SubscribeChainEvent(chainEvCh)
	)

	// Unsubscribe all events
	//defer sub.Unsubscribe()
	defer txSub.Unsubscribe()
	//	defer rmLogsSub.Unsubscribe()
	//	defer logsSub.Unsubscribe()
	//	defer chainEvSub.Unsubscribe()

	for i := UnknownSubscription; i < LastIndexSubscription; i++ {
		index[i] = make(map[rpc.ID]*subscription)
	}

	for {
		select {
		// Handle subscribed events
		case ev := <-txCh:
			es.broadcast(index, ev)
			//		case ev := <-rmLogsCh:
			//			es.broadcast(index, ev)
			//		case ev := <-logsCh:
			//			es.broadcast(index, ev)
			//		case ev := <-chainEvCh:
			//			es.broadcast(index, ev)

		case f := <-es.install:
			if f.typ == MinedAndPendingLogsSubscription {
				// the type are logs and pending logs subscriptions
				index[LogsSubscription][f.id] = f
				index[PendingLogsSubscription][f.id] = f
			} else {
				index[f.typ][f.id] = f
			}
			close(f.installed)
		case f := <-es.uninstall:
			if f.typ == MinedAndPendingLogsSubscription {
				// the type are logs and pending logs subscriptions
				delete(index[LogsSubscription], f.id)
				delete(index[PendingLogsSubscription], f.id)
			} else {
				delete(index[f.typ], f.id)
			}
			close(f.err)

		// System stopped
		case <-txSub.Err():
			return
			//		case <-rmLogsSub.Err():
			//			return
			//		case <-logsSub.Err():
			//			return
			//		case <-chainEvSub.Err():
			//			return
		}
	}

}
