/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consensus

import (
	"log"
	"os"

	"github.com/BeDreamCoder/wal"
	walcache "github.com/BeDreamCoder/wal/cache"
	walog "github.com/BeDreamCoder/wal/log"
	"github.com/BeDreamCoder/wal/log/walpb"
	"github.com/BeDreamCoder/wal/snap"
	"github.com/BeDreamCoder/wal/snap/snappb"
	"github.com/zhigui-projects/go-hotstuff/api"
	"go.etcd.io/etcd/pkg/fileutil"
	"go.uber.org/zap"
)

type hsWAL struct {
	logger  api.Logger
	commitC chan<- *string // entries committed to log (k,v)

	id        int    // node ID
	waldir    string // path to WAL directory
	snapdir   string // path to snapshot directory
	lastIndex uint64 // index of log at start

	walcache.MemoryStorage
	wal.Storage
}

func (hs *hsWAL) startNode() {
	if !fileutil.Exist(hs.snapdir) {
		if err := os.Mkdir(hs.snapdir, 0750); err != nil {
			hs.logger.Fatalf("cannot create dir for snapshot (%v)", err)
		}
	}

	oldwal := walog.Exist(hs.waldir)
	hs.Storage = wal.NewStorage(hs.replayWAL(), snap.New(zap.NewExample(), hs.snapdir))

	if oldwal {
	} else {
	}
}

// replayWAL replays WAL entries into the raft instance.
func (hs *hsWAL) replayWAL() *walog.WAL {
	hs.logger.Debugf("replaying WAL of member %d", hs.id)
	snapshot := hs.loadSnapshot()
	w := hs.openWAL(snapshot)
	_, st, ents, err := w.ReadAll()
	if err != nil {
		hs.logger.Fatalf("failed to read WAL (%v)", err)
	}
	hs.MemoryStorage = walcache.NewMemoryStorage()
	if snapshot != nil {
		hs.ApplySnapshot(*snapshot, ents)
	}
	hs.SetHardState(st)

	// append to storage so raft starts at the right place in log
	hs.Append(ents)
	// send nil once lastIndex is published so client knows commit channel is current
	if len(ents) > 0 {
		hs.lastIndex = ents[len(ents)-1].GetIndex()
	} else {
		//hs.commitC <- nil
	}
	return w
}

func (hs *hsWAL) loadSnapshot() *snappb.ShotData {
	snapshot, err := hs.Load()
	if err != nil && err != snap.ErrNoSnapshot {
		hs.logger.Fatalf("error loading snapshot (%v)", err)
	}
	return snapshot
}

// openWAL returns a WAL ready for reading.
func (hs *hsWAL) openWAL(snapshot *snappb.ShotData) *walog.WAL {
	if !walog.Exist(hs.waldir) {
		if err := os.Mkdir(hs.waldir, 0750); err != nil {
			hs.logger.Fatalf("cannot create dir for wal (%v)", err)
		}

		w, err := walog.Create(zap.NewExample(), hs.waldir, nil)
		if err != nil {
			log.Fatalf("create wal error (%v)", err)
		}
		w.Close()
	}

	walsnap := &walpb.Snapshot{}
	if snapshot != nil {
		walsnap.Index = snapshot.Index
	}
	hs.logger.Debugf("loading WAL at  index %d", walsnap.Index)
	w, err := walog.Open(zap.NewExample(), hs.waldir, walsnap)
	if err != nil {
		hs.logger.Fatalf("error loading wal (%v)", err)
	}
	return w
}
