// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------

package holochain

import (
	"fmt"
	"github.com/google/uuid"
	. "github.com/holochain/holochain-proto/hash"
)

type DNA struct {
	Version                   int
	UUID                      uuid.UUID
	Name                      string
	Properties                map[string]string
	PropertiesSchema          string
	AgentIdentitySchema       string // defines what must go in the Indentity field of a key/agent entry
	BasedOn                   Hash   // references hash of another holochain that these schemas and code are derived from
	RequiresVersion           int
	DHTConfig                 DHTConfig
	Progenitor                Progenitor
	Zomes                     []Zome
	propertiesSchemaValidator SchemaValidator
}

func (dna *DNA) check() (err error) {
	if dna.RequiresVersion > Version {
		err = fmt.Errorf("Chain requires Holochain version %d", dna.RequiresVersion)
	}
	return
}

// Nucleus encapsulates Application parts: Ribosomes to run code in Zomes, plus application
// validation and direct message passing protocols
type Nucleus struct {
	dna  *DNA
	h    *Holochain
	alog *Logger // the app logger
}

func (n *Nucleus) DNA() (dna *DNA) {
	return n.dna
}

// NewNucleus creates a new Nucleus structure
func NewNucleus(h *Holochain, dna *DNA) *Nucleus {
	nucleus := Nucleus{
		dna:  dna,
		h:    h,
		alog: &h.Config.Loggers.App,
	}
	return &nucleus
}

func (n *Nucleus) RunGenesis() (err error) {
	var ribosome Ribosome
	// run the init functions of each zome
	for _, zome := range n.dna.Zomes {
		ribosome, err = zome.MakeRibosome(n.h)
		if err == nil {
			err = ribosome.ChainGenesis()
			if err != nil {
				err = fmt.Errorf("In '%s' zome: %s", zome.Name, err.Error())
				return
			}
		}
	}
	return
}

func (n *Nucleus) Start() (err error) {
	h := n.h
	if err = h.node.StartProtocol(h, ValidateProtocol); err != nil {
		return
	}
	if err = h.node.StartProtocol(h, ActionProtocol); err != nil {
		return
	}
	return
}

type AppMsg struct {
	ZomeType string
	Body     string
}

// ActionReceiver handles messages on the action protocol
func ActionReceiver(h *Holochain, msg *Message) (response interface{}, err error) {
	return actionReceiver(h, msg, MaxRetries)
}

func isRelatedHoldMessage(msg *Message) bool {
	return msg.Type == MIGRATE_REQUEST || msg.Type == MOD_REQUEST || msg.Type == DEL_REQUEST || msg.Type == LINK_REQUEST
}

func actionReceiver(h *Holochain, msg *Message, retries int) (response interface{}, err error) {
	dht := h.dht
	// to protect against crashes from background routines after close
	if dht == nil {
		return
	}
	var a Action
	a, err = MakeActionFromMessage(msg)
	if err == nil {
		dht.dlog.Logf("ActionReceiver got %s: %v", a.Name(), msg)

		// If this is a Del/Mod/Link message then we need to check to see if we
		// have the Related Hash and if not, queue for retry
		if isRelatedHoldMessage(msg) {
			t := msg.Body.(HoldReq)
			err = dht.Exists(t.RelatedHash, StatusDefault)
			if err != nil {
				if err == ErrHashNotFound {
					dht.dlog.Logf("don't yet have %s, trying again later", t.RelatedHash)
					retry := &retry{msg: *msg, retries: retries}
					dht.retryQueue <- retry
					response = DHTChangeUnknownHashQueuedForRetry
					err = nil
				}
			}

			if response != nil || err != nil {
				return
			}
		}

		// N.B. a.Receive calls made to an Action whose values are NOT populated.
		// The Receive functions understand this and use the values from the message body
		// TODO, this indicates an architectural error, so fix!
		response, err = a.Receive(dht, msg)
	}
	return
}

// NewUUID generates a new UUID for the DNA
func (dna *DNA) NewUUID() (err error) {
	dna.UUID, err = uuid.NewUUID()
	if err != nil {
		return
	}
	return
}
