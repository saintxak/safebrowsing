// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package safebrowsing

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	google_protobuf "github.com/golang/protobuf/ptypes/duration"
	pb "github.com/saintxak/safebrowsing/internal/safebrowsing_proto"
)

func mustGetTempFile(t *testing.T) string {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return path
}

func mustDecodeHex(t *testing.T, s string) []byte {
	buf, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return buf
}

func newHashSet(phs hashPrefixes) (hs hashSet) {
	hs.Import(phs)
	return hs
}

func TestDatabaseInit(t *testing.T) {
	path := mustGetTempFile(t)
	defer os.Remove(path)

	now := time.Unix(1451436338, 951473000)
	mockNow := func() time.Time { return now }

	vectors := []struct {
		config *Config   // Input configuration
		oldDB  *database // The old database (before export)
		newDB  *database // The expected new database (after import)
		fail   bool      // Expected failure
	}{{
		// Load from a valid database file.
		config: &Config{
			ThreatLists: []ThreatDescriptor{
				{0, 2, 3},
				{1, 2, 3},
			},
			UpdatePeriod: DefaultUpdatePeriod,
		},
		oldDB: &database{
			last: now.Add(-DefaultUpdatePeriod + time.Minute),
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"aaaa", "bbbb"},
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
				{1, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"bbbb", "cccc"},
					SHA256: mustDecodeHex(t, "9a720c6ee500f5a0d4e5477fc9f3d8573226723d0b338b0c8f572d877bdfa224"),
					State:  []byte("state2"),
				},
			},
		},
		newDB: &database{
			last: now.Add(-DefaultUpdatePeriod + time.Minute),
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
				{1, 2, 3}: partialHashes{
					SHA256: mustDecodeHex(t, "9a720c6ee500f5a0d4e5477fc9f3d8573226723d0b338b0c8f572d877bdfa224"),
					State:  []byte("state2"),
				},
			},
			tfl: threatsForLookup{
				{0, 2, 3}: newHashSet([]hashPrefix{"aaaa", "bbbb"}),
				{1, 2, 3}: newHashSet([]hashPrefix{"bbbb", "cccc"}),
			},
		},
	}, {
		// Load from an older but not yet stale valid database file.
		config: &Config{
			ThreatLists: []ThreatDescriptor{
				{0, 2, 3},
				{1, 2, 3},
			},
			UpdatePeriod: DefaultUpdatePeriod,
		},
		oldDB: &database{
			last: now.Add(-DefaultUpdatePeriod + (30 * time.Minute)),
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"aaaa", "bbbb"},
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
				{1, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"bbbb", "cccc"},
					SHA256: mustDecodeHex(t, "9a720c6ee500f5a0d4e5477fc9f3d8573226723d0b338b0c8f572d877bdfa224"),
					State:  []byte("state2"),
				},
			},
		},
		newDB: &database{
			last: now.Add(-DefaultUpdatePeriod + (30 * time.Minute)),
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
				{1, 2, 3}: partialHashes{
					SHA256: mustDecodeHex(t, "9a720c6ee500f5a0d4e5477fc9f3d8573226723d0b338b0c8f572d877bdfa224"),
					State:  []byte("state2"),
				},
			},
			tfl: threatsForLookup{
				{0, 2, 3}: newHashSet([]hashPrefix{"aaaa", "bbbb"}),
				{1, 2, 3}: newHashSet([]hashPrefix{"bbbb", "cccc"}),
			},
		},
	}, {
		// Load from a valid database file with more descriptors than in configuration.
		config: &Config{
			ThreatLists: []ThreatDescriptor{{0, 2, 3}},
		},
		oldDB: &database{
			last: now,
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"aaaa", "bbbb"},
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
				{1, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"bbbb", "cccc"},
					SHA256: mustDecodeHex(t, "9a720c6ee500f5a0d4e5477fc9f3d8573226723d0b338b0c8f572d877bdfa224"),
					State:  []byte("state2"),
				},
				{3, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"xxx", "yyy", "zzz"},
					SHA256: mustDecodeHex(t, "cc6c955cadf2cc09442c0848ce8e165b8f9aa5974916de7186a9e1b6c4e7937e"),
				},
			},
		},
		newDB: &database{
			last: now,
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
			},
			tfl: threatsForLookup{
				{0, 2, 3}: newHashSet([]hashPrefix{"aaaa", "bbbb"}),
			},
		},
	}, {
		// Load from a invalid database file with fewer descriptors than in configuration.
		config: &Config{
			ThreatLists: []ThreatDescriptor{
				{0, 1, 3},
				{0, 2, 3},
				{1, 1, 3},
				{1, 2, 3},
			},
		},
		oldDB: &database{
			last: now,
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"aaaa", "bbbb"},
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
				{1, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"bbbb", "cccc"},
					SHA256: mustDecodeHex(t, "9a720c6ee500f5a0d4e5477fc9f3d8573226723d0b338b0c8f572d877bdfa224"),
					State:  []byte("state2"),
				},
				{0, 1, 3}: partialHashes{
					Hashes: []hashPrefix{"xxx", "yyy", "zzz"},
					SHA256: mustDecodeHex(t, "cc6c955cadf2cc09442c0848ce8e165b8f9aa5974916de7186a9e1b6c4e7937e"),
				},
			},
		},
		fail: true,
	}, {
		// Load from a stale database file.
		config: &Config{
			ThreatLists: []ThreatDescriptor{
				{0, 2, 3},
				{1, 2, 3},
			},
			UpdatePeriod: DefaultUpdatePeriod,
		},
		oldDB: &database{
			last: now.Add(-2 * (DefaultUpdatePeriod + time.Minute)),
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"aaaa", "bbbb"},
					SHA256: mustDecodeHex(t, "e5c1edb50ff8b4fcc3ead3a845ffbe1ad51c9dae5d44335a5c333b57ac8df062"),
					State:  []byte("state1"),
				},
				{1, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"bbbb", "cccc"},
					SHA256: mustDecodeHex(t, "9a720c6ee500f5a0d4e5477fc9f3d8573226723d0b338b0c8f572d877bdfa224"),
					State:  []byte("state2"),
				},
			},
		},
		fail: true,
	}, {
		// Load from a corrupted database file (has bad SHA256 checksums).
		config: &Config{
			ThreatLists: []ThreatDescriptor{
				{0, 2, 3},
				{1, 2, 3},
			},
		},
		oldDB: &database{
			last: now,
			tfu: threatsForUpdate{
				{0, 2, 3}: partialHashes{
					Hashes: []hashPrefix{"aaaa", "bbbb"},
					State:  []byte("state1"),
					SHA256: []byte("bad checksum"),
				},
			},
		},
		fail: true,
	}}

	logger := log.New(ioutil.Discard, "", 0)
	for i, v := range vectors {
		v.config.DBPath = path

		db1 := v.oldDB
		db1.config = v.config
		dbf := databaseFormat{db1.tfu, db1.last}
		if err := saveDatabase(db1.config.DBPath, dbf); err != nil {
			t.Errorf("test %d, unexpected save error: %v", i, err)
		}

		db2 := new(database)
		v.config.now = mockNow
		if fail := !db2.Init(v.config, logger); fail != v.fail {
			t.Errorf("test %d, mismatching status: got %v, want %v", i, fail, v.fail)
		}

		db2.config, db2.log, db2.readyCh = nil, nil, nil
		if !v.fail && !reflect.DeepEqual(db2, v.newDB) {
			t.Errorf("test %d, mismatching database contents:\ngot  %+v\nwant %+v", i, db2, v.newDB)
		}
	}
}

func TestDatabaseUpdate(t *testing.T) {
	var (
		td013 = ThreatDescriptor{0, 1, 3}
		td014 = ThreatDescriptor{0, 1, 4}
		td113 = ThreatDescriptor{1, 1, 3}
		td114 = ThreatDescriptor{1, 1, 4}

		full    = int(pb.FetchThreatListUpdatesResponse_ListUpdateResponse_FULL_UPDATE)
		partial = int(pb.FetchThreatListUpdatesResponse_ListUpdateResponse_PARTIAL_UPDATE)

		config = &Config{
			ThreatLists: []ThreatDescriptor{
				{0, 2, 3},
				{0, 2, 4},
				{1, 2, 3},
				{1, 2, 4},
			},
			UpdatePeriod: 1800 * time.Second,
		}
		logger = log.New(ioutil.Discard, "", 0)
	)

	// Helper function to aid in the construction on responses.
	newResp := func(td ThreatDescriptor, rtype int, dels []int32, adds []string, state string, chksum string) *pb.FetchThreatListUpdatesResponse_ListUpdateResponse {
		resp := &pb.FetchThreatListUpdatesResponse_ListUpdateResponse{
			ThreatType:      pb.ThreatType(td.ThreatType),
			PlatformType:    pb.PlatformType(td.PlatformType),
			ThreatEntryType: pb.ThreatEntryType(td.ThreatEntryType),
			ResponseType:    pb.FetchThreatListUpdatesResponse_ListUpdateResponse_ResponseType(rtype),
			NewClientState:  []byte(state),
			Checksum:        &pb.Checksum{Sha256: mustDecodeHex(t, chksum)},
		}
		if dels != nil {
			resp.Removals = []*pb.ThreatEntrySet{{
				CompressionType: pb.CompressionType_RAW,
				RawIndices:      &pb.RawIndices{Indices: dels},
			}}
		}
		if adds != nil {
			bySize := make(map[int][]string)
			for _, s := range adds {
				bySize[len(s)] = append(bySize[len(s)], s)
			}
			for n, hs := range bySize {
				sort.Strings(hs)
				resp.Additions = append(resp.Additions, &pb.ThreatEntrySet{
					CompressionType: pb.CompressionType_RAW,
					RawHashes: &pb.RawHashes{
						PrefixSize: int32(n),
						RawHashes:  []byte(strings.Join(hs, "")),
					},
				})
			}
		}
		return resp
	}

	// Setup mocking objects.
	var now time.Time
	mockNow := func() time.Time { return now }
	config.now = mockNow

	var resp pb.FetchThreatListUpdatesResponse
	var errResponse error
	mockAPI := &mockAPI{
		listUpdate: func(context.Context, *pb.FetchThreatListUpdatesRequest) (*pb.FetchThreatListUpdatesResponse, error) {
			return &resp, errResponse
		},
	}

	// Setup the database under test.
	var gotDB, wantDB *database
	db := &database{config: config, log: logger}

	// Update 0: partial update on empty database.
	now = now.Add(time.Hour)
	resp = pb.FetchThreatListUpdatesResponse{
		ListUpdateResponses: []*pb.FetchThreatListUpdatesResponse_ListUpdateResponse{
			newResp(td013, full, nil, nil,
				"a0", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			newResp(td113, full, nil, nil,
				"b0", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			newResp(td014, full, nil, nil,
				"c0", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			newResp(td114, partial, []int32{0, 1, 2, 3}, nil,
				"d0", "0000000000000000000000000000000000000000000000000000000000000000"),
		},
		MinimumWaitDuration: &google_protobuf.Duration{Seconds: 1200},
	}
	delay, updated := db.Update(context.Background(), mockAPI)
	if db.err == nil || updated {
		t.Fatalf("update 0, unexpected update success")
	}

	// MinimumWaitDuration is less than the jitter generated by the db code so don't use it
	if math.Abs((config.UpdatePeriod - delay).Seconds()) > 31 {
		t.Fatalf("update 0, jitter was more than 30 seconds")
	}

	// Update 1: full update to all values.
	now = now.Add(time.Hour)
	resp = pb.FetchThreatListUpdatesResponse{
		ListUpdateResponses: []*pb.FetchThreatListUpdatesResponse_ListUpdateResponse{
			newResp(td013, full, nil, []string{"aaaa", "bbbb", "cccc", "0421e", "0421f", "a64392f6f89487"},
				"a1", "6a2480447ff0715d5c28090c3333fba69bd918faad4e9e0f977f3cc76f422ad6"),
			newResp(td113, full, nil, nil,
				"b1", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			newResp(td014, full, nil, []string{"aaaa", "bbbb", "cccc", "dddd"},
				"c1", "147eb9dcde0e090429c01dbf634fd9b69a7f141f005c387a9c00498908499dde"),
			newResp(td114, full, nil, []string{"aaaa", "0421e", "666666", "7777777", "88888888"},
				"d1", "a3b93fac424834c2447e2dbe5db3ec8553519777523907ea310e207f556a7637"),
		},
		MinimumWaitDuration: &google_protobuf.Duration{Seconds: 2000},
	}
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err != nil || !updated {
		t.Fatalf("update 1, unexpected update error: %v", db.err)
	}

	// Make sure we respect the MinimumWaitDuration from the API
	expectedDelay := time.Duration(2000 * time.Second)
	if delay != expectedDelay {
		t.Fatalf("update 1, expected delay %v got %v", expectedDelay, delay)
	}

	gotDB = &database{last: db.last, tfu: db.tfu, tfl: db.tfl}
	wantDB = &database{
		last: now,
		tfu: threatsForUpdate{
			td013: {SHA256: gotDB.tfu[td013].SHA256, State: []byte{0x61, 0x31}},
			td113: {SHA256: gotDB.tfu[td113].SHA256, State: []byte{0x62, 0x31}},
			td014: {SHA256: gotDB.tfu[td014].SHA256, State: []byte{0x63, 0x31}},
			td114: {SHA256: gotDB.tfu[td114].SHA256, State: []byte{0x64, 0x31}},
		},
		tfl: threatsForLookup{
			td013: newHashSet([]hashPrefix{"0421e", "0421f", "a64392f6f89487", "aaaa", "bbbb", "cccc"}),
			td113: newHashSet([]hashPrefix{}),
			td014: newHashSet([]hashPrefix{"aaaa", "bbbb", "cccc", "dddd"}),
			td114: newHashSet([]hashPrefix{"0421e", "666666", "7777777", "88888888", "aaaa"}),
		},
	}
	if !reflect.DeepEqual(gotDB.tfu, wantDB.tfu) {
		t.Errorf("update 1, threats for update mismatch:\ngot  %+v\nwant %+v", gotDB.tfu, wantDB.tfu)
	}
	if !reflect.DeepEqual(gotDB.tfl, wantDB.tfl) {
		t.Fatalf("update 1, threats for lookup mismatch:\ngot  %+v\nwant %+v", gotDB.tfl, wantDB.tfl)
	}

	// Update 2: partial update with no changes.
	now = now.Add(time.Hour)
	resp = pb.FetchThreatListUpdatesResponse{
		ListUpdateResponses: []*pb.FetchThreatListUpdatesResponse_ListUpdateResponse{
			newResp(td013, partial, nil, nil,
				"a1", "6a2480447ff0715d5c28090c3333fba69bd918faad4e9e0f977f3cc76f422ad6"),
			newResp(td113, partial, nil, nil,
				"b1", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			newResp(td014, partial, nil, nil,
				"c1", "147eb9dcde0e090429c01dbf634fd9b69a7f141f005c387a9c00498908499dde"),
			newResp(td114, partial, nil, nil,
				"d1", "a3b93fac424834c2447e2dbe5db3ec8553519777523907ea310e207f556a7637"),
		},
	}
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err != nil || !updated {
		t.Fatalf("update 2, unexpected update error: %v", db.err)
	}
	if math.Abs((config.UpdatePeriod - delay).Seconds()) > 31 {
		t.Fatalf("update 2, delay jitter was more than 30 seconds")
	}
	gotDB = &database{last: db.last, tfu: db.tfu, tfl: db.tfl}
	wantDB.last = now
	if !reflect.DeepEqual(gotDB.tfu, wantDB.tfu) {
		t.Errorf("update 2, threats for update mismatch:\ngot  %+v\nwant %+v", gotDB.tfu, wantDB.tfu)
	}
	if !reflect.DeepEqual(gotDB.tfl, wantDB.tfl) {
		t.Fatalf("update 2, threats for lookup mismatch:\ngot  %+v\nwant %+v", gotDB.tfl, wantDB.tfl)
	}

	// Update 3: full update and partial update with removals and additions.
	now = now.Add(time.Hour)
	resp = pb.FetchThreatListUpdatesResponse{
		ListUpdateResponses: []*pb.FetchThreatListUpdatesResponse_ListUpdateResponse{
			newResp(td013, partial, []int32{0, 2, 4}, nil,
				"a2", "bd3291e0b4fc7522ee3363376ded7801b316184722d83d224948889dfcf12465"),
			newResp(td113, partial, nil, []string{"aaaa", "bbbb", "cccc"},
				"b2", "11c85195ae99540ac07f80e2905e6e39aaefc4ac94cd380f366e79ba83560566"),
			newResp(td014, partial, []int32{1, 3}, []string{"eeee", "ffff"},
				"c2", "d8b19bc1d972cae450d65494565d2c04653894618faf9b37148e2c78ea3807e5"),
			newResp(td114, full, nil, []string{"AAAA", "0421E"},
				"d2", "b742965b7a759ba0254685bfc6edae3b1ba54d0168fb86f526d6c79c3d44c753"),
		},
	}
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err != nil || !updated {
		t.Fatalf("update 3, unexpected update error: %v", db.err)
	}
	if math.Abs((config.UpdatePeriod - delay).Seconds()) > 31 {
		t.Fatalf("update 3, delay jitter was more than 30 seconds")
	}
	gotDB = &database{last: db.last, tfu: db.tfu, tfl: db.tfl}
	wantDB = &database{
		last: now,
		tfu: threatsForUpdate{
			td013: {SHA256: gotDB.tfu[td013].SHA256, State: []byte{0x61, 0x32}},
			td113: {SHA256: gotDB.tfu[td113].SHA256, State: []byte{0x62, 0x32}},
			td014: {SHA256: gotDB.tfu[td014].SHA256, State: []byte{0x63, 0x32}},
			td114: {SHA256: gotDB.tfu[td114].SHA256, State: []byte{0x64, 0x32}},
		},
		tfl: threatsForLookup{
			td013: newHashSet([]hashPrefix{"0421f", "aaaa", "cccc"}),
			td113: newHashSet([]hashPrefix{"aaaa", "bbbb", "cccc"}),
			td014: newHashSet([]hashPrefix{"aaaa", "cccc", "eeee", "ffff"}),
			td114: newHashSet([]hashPrefix{"0421E", "AAAA"}),
		},
	}
	if !reflect.DeepEqual(gotDB.tfu, wantDB.tfu) {
		t.Errorf("update 3, threats for update mismatch:\ngot  %+v\nwant %+v", gotDB.tfu, wantDB.tfu)
	}
	if !reflect.DeepEqual(gotDB.tfl, wantDB.tfl) {
		t.Fatalf("update 3, threats for lookup mismatch:\ngot  %+v\nwant %+v", gotDB.tfl, wantDB.tfl)
	}

	// Update 4: invalid SHA256 checksum.
	now = now.Add(time.Hour)
	resp = pb.FetchThreatListUpdatesResponse{
		ListUpdateResponses: []*pb.FetchThreatListUpdatesResponse_ListUpdateResponse{
			newResp(td013, partial, []int32{0, 1}, []string{"fizz", "buzz"},
				"a3", "bad0bad0bad0bad0bad0bad0bad0bad0bad0bad0bad0bad0bad0bad0bad0bad0"),
		},
	}
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err == nil || updated {
		t.Fatalf("update 4, unexpected update success")
	}
	if math.Abs((config.UpdatePeriod - delay).Seconds()) > 31 {
		t.Fatalf("update 4, delay jitter was more than 30 seconds")
	}
	gotDB = &database{last: db.last, tfu: db.tfu, tfl: db.tfl}
	wantDB = &database{}
	if !reflect.DeepEqual(gotDB, wantDB) {
		t.Fatalf("update 4, database state mismatch:\ngot  %+v\nwant %+v", gotDB, wantDB)
	}

	// Update 5: removal index is out-of-bounds.
	now = now.Add(time.Hour)
	resp = pb.FetchThreatListUpdatesResponse{
		ListUpdateResponses: []*pb.FetchThreatListUpdatesResponse_ListUpdateResponse{
			newResp(td013, partial, []int32{9000}, []string{"fizz", "buzz"},
				"a4", "5d6506974928a003d2a0ccbd7a40b5341ad10578fd3f54527087c5ecbbd17a12"),
		},
	}
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err == nil || updated {
		t.Fatalf("update 5, unexpected update success")
	}
	if math.Abs((config.UpdatePeriod - delay).Seconds()) > 31 {
		t.Fatalf("update 5, delay jitter was more than 30 seconds")
	}
	gotDB = &database{last: db.last, tfu: db.tfu, tfl: db.tfl}
	wantDB = &database{}
	if !reflect.DeepEqual(gotDB, wantDB) {
		t.Fatalf("update 5, database state mismatch:\ngot  %+v\nwant %+v", gotDB, wantDB)
	}

	// Update 6: api is broken for some unknown reason. Checks the backoff
	errResponse = errors.New("Something broke")
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err == nil || updated {
		t.Fatalf("update 6, unexpected update success")
	}
	minDelay := baseRetryDelay.Seconds() * float64(1) * float64(1)
	maxDelay := baseRetryDelay.Seconds() * float64(2) * float64(1)
	if delay.Seconds() < minDelay || delay.Seconds() > maxDelay {
		t.Fatalf("update 6, Expected delay %v to be between %v and %v", delay.Seconds(), minDelay, maxDelay)
	}

	// Update 7: api is still broken, check backoff is larger
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err == nil || updated {
		t.Fatalf("update 7, unexpected update success")
	}
	minDelay = baseRetryDelay.Seconds() * float64(1) * float64(2)
	maxDelay = baseRetryDelay.Seconds() * float64(2) * float64(2)
	if delay.Seconds() < minDelay || delay.Seconds() > maxDelay {
		t.Fatalf("update 7, Expected delay %v to be between %v and %v", delay.Seconds(), minDelay, maxDelay)
	}

	// Update 8: api is still broken, check that backoff is larger than before
	delay, updated = db.Update(context.Background(), mockAPI)
	if db.err == nil || updated {
		t.Fatalf("update 8, unexpected update success")
	}
	minDelay = baseRetryDelay.Seconds() * float64(1) * float64(4)
	maxDelay = baseRetryDelay.Seconds() * float64(2) * float64(4)
	if delay.Seconds() < minDelay || delay.Seconds() > maxDelay {
		t.Fatalf("update 8, Expected delay %v to be between %v and %v", delay.Seconds(), minDelay, maxDelay)
	}
}

func TestDatabaseLookup(t *testing.T) {
	var (
		td000 = ThreatDescriptor{0, 0, 0}
		td001 = ThreatDescriptor{0, 0, 1}
		td012 = ThreatDescriptor{0, 1, 2}
		td123 = ThreatDescriptor{1, 2, 3}
		td234 = ThreatDescriptor{2, 3, 4}
		td456 = ThreatDescriptor{4, 5, 6}
		td567 = ThreatDescriptor{5, 6, 7}
		td678 = ThreatDescriptor{6, 7, 8}
	)

	threatsEqual := func(a, b []ThreatDescriptor) bool {
		ma := make(map[ThreatDescriptor]struct{})
		mb := make(map[ThreatDescriptor]struct{})
		for _, td := range a {
			ma[td] = struct{}{}
		}
		for _, td := range b {
			mb[td] = struct{}{}
		}
		return reflect.DeepEqual(ma, mb)
	}

	db := &database{tfl: threatsForLookup{
		td000: newHashSet([]hashPrefix{
			"26e307", "524d", "5c6655d4"}),
		td001: newHashSet([]hashPrefix{
			"26e307", "3f93", "5c6655d4", "7294"}),
		td012: newHashSet([]hashPrefix{
			"3f93", "5c6655d3", "5c6655d4", "7294"}),
		td123: newHashSet([]hashPrefix{
			"1e25395a9b1b8", "3f93", "5c6655d2", "5c6655d5", "7294", "cad78c1c"}),
		td234: newHashSet([]hashPrefix{
			"1e25395a9b1b8", "cad78c628", "cad78c68"}),
		td456: newHashSet([]hashPrefix{
			"524d", "59b8", "5c6655d3", "cad78c1c"}),
		td567: newHashSet([]hashPrefix{
			"1e25395a9b1b8", "26e307", "524d", "5c6655d5", "7294", "cad78c1c"}),
		td678: newHashSet([]hashPrefix{
			"1e25395a9b1b8", "524d", "524d", "7294", "cad78c628"}),
	}}

	vectors := []struct {
		input   hashPrefix // Input full hash
		output  hashPrefix // Output partial hash
		threats []ThreatDescriptor
	}{{
		input:  "3db40718dad209613a1fd9dced74dc0e",
		output: "", // Not found
	}, {
		input:   "59b8332112b29950f594cf957f4d0e63",
		output:  "59b8",
		threats: []ThreatDescriptor{td456},
	}, {
		input:   "524dfa307ba397754a35dcce0ee5f54a",
		output:  "524d",
		threats: []ThreatDescriptor{td000, td678, td456, td678, td567},
	}, {
		input:   "524dea307ba397754a35dcce0ee5f54a",
		output:  "524d",
		threats: []ThreatDescriptor{td000, td678, td456, td678, td567},
	}, {
		input:   "5c6655d2096dd9ffb3c9e2bd5f86798f",
		output:  "5c6655d2",
		threats: []ThreatDescriptor{td123},
	}, {
		input:   "5c6655d33db40718dad209613a1fd9dc",
		output:  "5c6655d3",
		threats: []ThreatDescriptor{td012, td456},
	}, {
		input:   "1e25395a9b1b87db129a7d85ee7cc0fd",
		output:  "1e25395a9b1b8",
		threats: []ThreatDescriptor{td123, td234, td567, td678},
	}}

	for i, v := range vectors {
		ph, m := db.Lookup(v.input)
		if ph != v.output {
			t.Errorf("test %d, partial hash mismatch: got %s, want %s", i, ph, v.output)
		}
		if !threatsEqual(m, v.threats) {
			t.Errorf("test %d, results mismatch: got %v, want %v", i, m, v.threats)
		}
	}
}

func TestDatabasePersistence(t *testing.T) {
	path := mustGetTempFile(t)
	defer os.Remove(path)

	vectors := []struct {
		last time.Time        // Input last update time
		tfu  threatsForUpdate // Input threatsByDescriptor
	}{{
		last: time.Time{},
	}, {
		last: time.Now().Round(0), // Strip monotonic timestamp in Go1.9
	}, {
		last: time.Unix(123456, 789),
		tfu: threatsForUpdate{
			{0, 1, 2}: partialHashes{
				SHA256: mustDecodeHex(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
			},
		},
	}, {
		last: time.Unix(987654321, 0),
		tfu: threatsForUpdate{
			{3, 4, 5}: partialHashes{
				Hashes: []hashPrefix{"aaaa", "bbbb", "cccc", "dddd"},
				State:  []byte("meow meow meow!!!"),
				SHA256: mustDecodeHex(t, "147eb9dcde0e090429c01dbf634fd9b69a7f141f005c387a9c00498908499dde"),
			},
			{7, 8, 9}: partialHashes{
				Hashes: []hashPrefix{"xxxx", "yyyy", "zzzz"},
				State:  []byte("rawr rawr rawr!!!"),
				SHA256: mustDecodeHex(t, "20ffb2c3e9532153b96b956845381adc06095f8342fa2db1aafba6b0e9594d68"),
			},
		},
	}}

	for i, v := range vectors {
		dbf1 := databaseFormat{v.tfu, v.last}
		if err := saveDatabase(path, dbf1); err != nil {
			t.Errorf("test %d, unexpected save error: %v", i, err)
			continue
		}

		dbf2, err := loadDatabase(path)
		if err != nil {
			t.Errorf("test %d, unexpected load error: %v", i, err)
			continue
		}

		if !reflect.DeepEqual(dbf1, dbf2) {
			t.Errorf("test %d, mismatching database contents:\ngot  %v\nwant %v", i, dbf2, dbf1)
		}
	}
}

func TestDatabaseSaveErrors(t *testing.T) {
	path := mustGetTempFile(t)
	defer os.Remove(path)

	// Set mode to be unwritable.
	if err := os.Chmod(path, 0444); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := saveDatabase(path, databaseFormat{}); err == nil {
		t.Errorf("unexpected save success")
	}
}

func TestDatabaseLoadErrors(t *testing.T) {
	path := mustGetTempFile(t)
	defer os.Remove(path)

	dbf1 := databaseFormat{
		Table: threatsForUpdate{
			{3, 4, 5}: partialHashes{
				Hashes: []hashPrefix{"aaaa", "bbbb", "cccc", "dddd"},
				State:  []byte("meow meow meow!!!"),
				SHA256: nil, // Intentionally leave this out
			},
		},
	}
	if err := saveDatabase(path, dbf1); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := loadDatabase(path); err == nil {
		t.Errorf("unexpected success")
	}

	if err := os.Truncate(path, 13); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := loadDatabase(path); err != io.ErrUnexpectedEOF {
		t.Errorf("mismatching error: got %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

func TestReady(t *testing.T) {
	config := &Config{
		ThreatLists: []ThreatDescriptor{{0, 2, 3}},
	}

	db := new(database)
	logger := log.New(ioutil.Discard, "", 0)
	db.Init(config, logger)
	// Expect timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	select {
	case <-db.Ready():
		t.Fatal("db.Ready() is closed, wanted timeout")
	case <-ctx.Done():
		// expected
	}

	done := make(chan bool)
	go func(t *testing.T, done chan bool) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		select {
		case <-db.Ready():
			// expected
		case <-ctx.Done():
			t.Errorf("db.Ready() was not closed, expected close before timeout")
		}
		close(done)
	}(t, done)
	db.clearError()
	<-done

}

func TestIsStale(t *testing.T) {
	now := time.Unix(1451436338, 951473000)
	mockNow := func() time.Time { return now }
	db := new(database)
	logger := log.New(ioutil.Discard, "", 0)

	config := &Config{
		UpdatePeriod: DefaultUpdatePeriod,
	}
	db.Init(config, logger)
	db.config.now = mockNow

	vectors := []struct {
		LastUpdate    time.Time
		ExpectedStale bool
	}{
		// Last update of now isn't stale
		{LastUpdate: now, ExpectedStale: false},
		// Last update between DefaultUpdatePeriod and 2*DefaultUpdatePeriod isn't stale
		{LastUpdate: now.Add(-(DefaultUpdatePeriod + time.Minute)), ExpectedStale: false},
		// Last update right at the cusp of -2 * the DefaultUpdatePeriod isn't stale
		{LastUpdate: now.Add(-2 * DefaultUpdatePeriod), ExpectedStale: false},
		// Last update right past -2 * DefaultUpdatePeriod + jitter is stale
		{LastUpdate: now.Add(-2 * (DefaultUpdatePeriod + (2 * time.Minute))), ExpectedStale: true},
		// Last update well past -2 * DefaultUpdatePeriod + jitter is stale
		{LastUpdate: now.Add(-3 * (DefaultUpdatePeriod + time.Minute)), ExpectedStale: true},
	}

	for i, v := range vectors {
		stale := db.isStale(v.LastUpdate)
		if stale != v.ExpectedStale {
			t.Errorf("test %d, mismatching isStale: got %v, want %v", i, stale, v.ExpectedStale)
		}
	}
}
