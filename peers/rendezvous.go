package peers

import (
	"context"
	"crypto/ecdsa"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/ethereum/go-ethereum/p2p/enr"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/status-im/rendezvous"
)

const (
	registrationPeriod = 10 * time.Second
	bucketSize         = 10
)

func NewRendezvous(srv ma.Multiaddr, identity *ecdsa.PrivateKey, node *discover.Node) (*Rendezvous, error) {
	r := new(Rendezvous)
	r.srv = srv
	r.registrationPeriod = registrationPeriod
	r.bucketSize = bucketSize

	r.record = enr.Record{}
	r.record.Set(enr.IP(node.IP))
	r.record.Set(enr.TCP(node.TCP))
	r.record.Set(enr.UDP(node.UDP))
	if err := enr.SignV4(&r.record, identity); err != nil {
		return nil, err
	}
	return r, nil
}

// Rendezvous is an implementation of discovery interface that uses
// rendezvous client.
type Rendezvous struct {
	mu     sync.RWMutex
	client *rendezvous.Client

	srv                ma.Multiaddr
	registrationPeriod time.Duration
	bucketSize         int
	record             enr.Record
}

func (r *Rendezvous) Running() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client != nil
}

// Start creates client with temporary (not persisted) identity.
func (r *Rendezvous) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	client, err := rendezvous.NewTemporary()
	if err != nil {
		return err
	}
	r.client = &client
	return nil
}

// Stop removes client reference.
func (r *Rendezvous) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.client = nil
	return nil
}

// Register renews registration in the specified server.
func (r *Rendezvous) Register(topic string, stop chan struct{}) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ticker := time.NewTicker(r.registrationPeriod)
	defer ticker.Stop()
	register := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := r.client.Register(ctx, r.srv, topic, r.record)
		cancel()
		if err != nil {
			log.Error("error registering", "topic", topic, "rendevous server", r.srv, "err", err)
		}
	}
	register()
	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			register()
		}
	}
}

// Discover will search for new records every time period fetched from period channel.
func (r *Rendezvous) Discover(
	topic string, period <-chan time.Duration,
	found chan<- *discv5.Node, lookup chan<- bool) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	timePeriod := <-period
	ticker := time.NewTicker(timePeriod)
	for {
		select {
		case new, ok := <-period:
			ticker.Stop()
			if !ok {
				return nil
			}
			timePeriod = new
			ticker = time.NewTicker(new)
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			records, err := r.client.Discover(ctx, r.srv, topic, r.bucketSize)
			cancel()
			if err != nil {
				log.Error("error fetching records", "topic", topic, "rendezvous server", r.srv, "err", err)
			}
			for i := range records {
				n, err := enrToNode(records[i])
				if err != nil {
					log.Error("error converting enr record to node", "err", err)
				}
				found <- n
			}
		}
	}
}

func enrToNode(record enr.Record) (*discv5.Node, error) {
	var (
		key   enr.Secp256k1
		ip    enr.IP
		tport enr.TCP
		uport enr.UDP
	)
	if err := record.Load(&key); err != nil {
		return nil, err
	}
	if err := record.Load(&ip); err != nil {
		return nil, err
	}
	if err := record.Load(&tport); err != nil {
		return nil, err
	}
	if err := record.Load(&uport); err != nil {
		uport = enr.UDP(tport)
	}
	ecdsaKey := ecdsa.PublicKey(key)
	return discv5.NewNode(discv5.PubkeyID(&ecdsaKey), net.IP(ip), uint16(uport), uint16(tport)), nil
}
