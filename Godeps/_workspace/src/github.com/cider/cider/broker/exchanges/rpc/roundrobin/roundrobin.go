// Copyright (c) 2013 The cider AUTHORS
//
// Use of this source code is governed by The MIT License
// that can be found in the LICENSE file.

package roundrobin

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"

	"github.com/cider/cider/broker/log"
	"github.com/cider/cider/broker/services/rpc"
)

//------------------------------------------------------------------------------
// Balancer implements rpc.Exchange
//------------------------------------------------------------------------------

type Balancer struct {
	// app name -> app record
	apps map[string]*appRecord
	// method name -> providers
	providers map[string]*providersRecord
	// endpoint -> set of apps
	appsByEndpoint map[rpc.Endpoint]map[string]bool

	mu *sync.Mutex
}

func NewBalancer() rpc.Exchange {
	return &Balancer{
		apps:           make(map[string]*appRecord),
		providers:      make(map[string]*providersRecord),
		appsByEndpoint: make(map[rpc.Endpoint]map[string]bool),
		mu:             new(sync.Mutex),
	}
}

// Method management -----------------------------------------------------------

type appRecord struct {
	name             string
	rawName          []byte
	endpoint         rpc.Endpoint
	methods          map[string]bool
	requestReceivers map[uint16]*appRecord
}

type providersRecord struct {
	providers []*appRecord
	lastUsed  int
}

func (record *providersRecord) nextProvider() *appRecord {
	i := (record.lastUsed + 1) % len(record.providers)
	record.lastUsed = i
	return record.providers[i]
}

func (b *Balancer) RegisterMethod(appName string, endpoint rpc.Endpoint, method string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get the relevant app record.
	app := b.registerApp(appName, endpoint)

	// Make sure the method is not already registered.
	if _, ok := app.methods[method]; ok {
		log.Warnf("RoundRobin: method %s already exported by %s", method, appName)
		return ErrMethodAlreadyRegistered
	}

	app.methods[method] = true

	// Register the app as a provider for the given method.
	if record, ok := b.providers[method]; ok {
		record.providers = append(record.providers, app)
	} else {
		b.providers[method] = &providersRecord{
			providers: []*appRecord{app},
		}
	}

	log.Debugf("RoundRobin: method %q registered by %s", method, appName)
	return nil
}

func (b *Balancer) UnregisterMethod(appName string, method string) {
	b.mu.Lock()
	b.unregMethod(appName, method)
	b.mu.Unlock()
}

func (b *Balancer) unregMethod(appName string, method string) {
	// Delete the method from the app record.
	app := b.apps[appName]
	if app == nil {
		log.Warnf("RoundRobin: unregMethod: app %s not found", appName)
		return
	}
	delete(app.methods, method)

	// Remove the app from the set of providers for the given method.
	if record, ok := b.providers[method]; ok {
		for i, pi := range record.providers {
			if pi.name == appName {
				if len(record.providers) == 1 {
					delete(b.providers, method)
					return
				}
				record.providers = append(record.providers[:i], record.providers[i+1:]...)
				return
			}
		}
	}
}

func (b *Balancer) registerApp(appName string, srcEndpoint rpc.Endpoint) *appRecord {
	app := b.apps[appName]
	if app == nil {
		log.Infof("RoundRobin: application %s connected", appName)
		// If the app is not registered yet, create the record and ...
		app = &appRecord{
			name:             appName,
			rawName:          []byte(appName),
			endpoint:         srcEndpoint,
			methods:          make(map[string]bool, 1),
			requestReceivers: make(map[uint16]*appRecord),
		}
		b.apps[appName] = app
		// ... register the app with its endpoint.
		if apps, ok := b.appsByEndpoint[srcEndpoint]; ok {
			apps[appName] = true
		} else {
			b.appsByEndpoint[srcEndpoint] = map[string]bool{
				appName: true,
			}
		}
	}

	return app
}

func (b *Balancer) UnregisterApp(appName string) {
	b.mu.Lock()
	b.unregApp(appName)
	log.Infof("RoundRobin: application %s disconnected", appName)
	b.mu.Unlock()
}

func (b *Balancer) unregApp(appName string) {
	app := b.apps[appName]
	if app == nil {
		log.Warnf("RoundRobin: unregApp: app %s not found", appName)
		return
	}
	// Remove the app from the method providers index.
	for m := range app.methods {
		b.unregMethod(appName, m)
	}
	// Remove the app record.
	delete(b.apps, appName)
	// Remove the app from the app by endpoint index.
	delete(b.appsByEndpoint[app.endpoint], app.name)
}

func (b *Balancer) UnregisterEndpoint(endpoint rpc.Endpoint) {
	b.mu.Lock()
	defer b.mu.Unlock()
	apps := b.appsByEndpoint[endpoint]
	if apps == nil {
		return
	}
	for appName := range apps {
		b.unregApp(appName)
	}
}

// RPC routing -----------------------------------------------------------------

func (b *Balancer) HandleRequest(msg rpc.Request, srcEndpoint rpc.Endpoint) {
	b.mu.Lock()

	// Make sure the sender has a valid record.
	sender := string(msg.Sender())
	b.registerApp(sender, srcEndpoint)

	// Get the next provider.
	if providers, ok := b.providers[string(msg.Method())]; ok {
		app := providers.nextProvider()

		var reqId uint16
		err := binary.Read(bytes.NewReader(msg.Id()), binary.BigEndian, &reqId)
		if err != nil {
			b.mu.Unlock()
			return
		}

		// Save the mapping of sender x request ID -> processing app.
		b.regOutboundRequest(sender, reqId, app.name)

		// Relay the request to the chosen provider.
		err = app.endpoint.DispatchRequest(app.rawName, msg)
		if err != nil {
			msg.Reject(255, "Failed to dispatch request: "+err.Error())
		}
	} else {
		msg.Reject(254, "No method available")
	}
	b.mu.Unlock()
}

func (b *Balancer) HandleInterrupt(msg rpc.Interrupt) {
	b.mu.Lock()
	var reqId uint16
	err := binary.Read(bytes.NewReader(msg.TargetRequestId()), binary.BigEndian, &reqId)
	if err != nil {
		b.mu.Unlock()
		return
	}

	// Load the processing app for the given sender and request ID.
	if app := b.appProcessingGivenRequest(string(msg.Sender()), reqId); app != nil {
		// Relay the interrupt to the app processing the request.
		app.endpoint.DispatchInterrupt(app.rawName, msg)
	}
	b.mu.Unlock()
}

func (b *Balancer) HandleProgress(msg rpc.Progress) {
	b.mu.Lock()
	if app := b.apps[string(msg.Receiver())]; app != nil {
		app.endpoint.DispatchProgress(msg)
	}
	b.mu.Unlock()
}

func (b *Balancer) HandleStreamFrame(msg rpc.StreamFrame) {
	b.mu.Lock()
	if app := b.apps[string(msg.Receiver())]; app != nil {
		app.endpoint.DispatchStreamFrame(msg)
	}
	b.mu.Unlock()
}

func (b *Balancer) HandleReply(msg rpc.Reply) {
	b.mu.Lock()
	receiver := string(msg.Receiver())
	if app := b.apps[receiver]; app != nil {
		var reqId uint16
		err := binary.Read(bytes.NewReader(msg.TargetRequestId()), binary.BigEndian, &reqId)
		if err != nil {
			b.mu.Unlock()
			return
		}

		// Delete the request being resolved from the routing table.
		b.unregOutboundRequest(receiver, reqId)

		// Relay the reply to the requester.
		app.endpoint.DispatchReply(msg)
	}
	b.mu.Unlock()
}

// Modifying and querying the database -----------------------------------------

func (b *Balancer) appProcessingGivenRequest(sender string, reqId uint16) *appRecord {
	app := b.apps[sender]
	if app == nil {
		return nil
	}

	return app.requestReceivers[reqId]
}

func (b *Balancer) regOutboundRequest(sender string, reqId uint16, receiver string) {
	senderApp := b.apps[sender]
	if senderApp == nil {
		return
	}

	receiverApp := b.apps[receiver]
	if receiverApp == nil {
		log.Warnf("RoundRobin: reqOutboundRequest: receiver %s not found", sender)
		return
	}

	if _, ok := senderApp.requestReceivers[reqId]; ok {
		log.Warnf("RoundRobin: reqOutboundRequest: request ID already registered", sender)
		return
	}

	senderApp.requestReceivers[reqId] = receiverApp
}

func (b *Balancer) unregOutboundRequest(sender string, reqId uint16) {
	app := b.apps[sender]
	if app == nil {
		log.Warnf("RoundRobin: unreqOutboundRequest: app %s not found", sender)
		return
	}

	delete(app.requestReceivers, reqId)
}

// Errors ----------------------------------------------------------------------

var ErrMethodAlreadyRegistered = errors.New("Method already registered")
