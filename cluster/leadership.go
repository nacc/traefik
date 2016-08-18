package cluster

import (
	log "github.com/Sirupsen/logrus"
	"github.com/cenkalti/backoff"
	"github.com/containous/traefik/safe"
	"github.com/containous/traefik/types"
	"github.com/docker/leadership"
	"golang.org/x/net/context"
	"time"
)

// Leadership allows leadership election using a KV store
type Leadership struct {
	*safe.Pool
	*types.Cluster
	candidate   *leadership.Candidate
	baseContext context.Context
}

// NewLeadership creates a leadership
func NewLeadership(ctx context.Context, cluster *types.Cluster) *Leadership {
	return &Leadership{
		baseContext: ctx,
		Pool:        safe.NewPool(ctx),
		Cluster:     cluster,
		candidate:   leadership.NewCandidate(cluster.Store, cluster.Store.Prefix+"/leader", cluster.Node, 20*time.Second),
	}
}

// Participate tries to be a leader
func (l *Leadership) Participate() {
	safe.Go(func() {
		backOff := backoff.NewExponentialBackOff()
		operation := func() error {
			return l.run(l.candidate, l.baseContext)
		}

		notify := func(err error, time time.Duration) {
			log.Errorf("Leadership election error %+v, retrying in %s", err, time)
		}
		err := backoff.RetryNotify(operation, backOff, notify)
		if err != nil {
			log.Errorf("Cannot elect leadership %+v", err)
		}
	})
}

// Resign resigns from being a leader
func (l *Leadership) Resign() {
	log.Infof("Node %s resigning", l.Cluster.Node)
	l.candidate.Resign()
}

func (l *Leadership) run(candidate *leadership.Candidate, ctx context.Context) error {
	electedCh, errCh := candidate.RunForElection()
	log.Debugf("Node %s running for election", l.Cluster.Node)
	defer log.Debugf("Node %s no more running for election", l.Cluster.Node)
	for {
		select {
		case elected := <-electedCh:
			l.onElection(elected)
		case err := <-errCh:
			return err
		case <-ctx.Done():
			l.candidate.Resign()
			return nil
		}
	}
}

func (l *Leadership) onElection(elected bool) {
	if elected {
		log.Infof("Node %s elected leader", l.Cluster.Node)
		l.Start()
	} else {
		log.Infof("Node %s elected slave", l.Cluster.Node)
		l.Stop()
	}
}
