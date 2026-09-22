package downloader

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"explo/src/config"
	"explo/src/models"
)

type testMonitor struct {
	bytesPerPoll             int
	stall                    time.Duration
	statuses                 []string
	polls, retries, cleanups int
	moveErr                  error
	max                      time.Duration
}

func (m *testMonitor) GetConf() (MonitorConfig, error) {
	max := m.max
	if max == 0 {
		max = time.Hour
	}
	stall := m.stall
	if stall == 0 {
		stall = time.Hour
	}
	return MonitorConfig{CheckInterval: time.Millisecond, StallDuration: stall, MaxDuration: max, MigrateDownload: true, Service: "test"}, nil
}
func (m *testMonitor) GetDownloadStatus(tracks []*models.Track) (map[string]FileStatus, error) {
	i := m.polls
	m.polls++
	if i >= len(m.statuses) {
		i = len(m.statuses) - 1
	}
	state := m.statuses[i]
	result := map[string]FileStatus{}
	if state != "missing" {
		result[tracks[0].ID] = FileStatus{State: state, BytesTransferred: m.bytesPerPoll * m.polls, BytesRemaining: 1000, Filename: `share\album\song.mp3`}
	}
	return result, nil
}
func (m *testMonitor) MoveDownload(_, _, _ string, _ *models.Track) error { return m.moveErr }
func (m *testMonitor) Cleanup(models.Track, string) error                 { m.cleanups++; return nil }
func (m *testMonitor) RetryDownload(*models.Track) (bool, error) {
	m.retries++
	return m.retries == 1, nil
}

func TestImportFailureRemainsEligibleForFallback(t *testing.T) {
	track := &models.Track{ID: "id", CleanTitle: "song"}
	m := &testMonitor{statuses: []string{"Succeeded"}, moveErr: errors.New("missing source")}
	if err := (&DownloadClient{}).MonitorDownloads([]*models.Track{track}, m); err != nil {
		t.Fatal(err)
	}
	if track.Present || m.cleanups != 0 || track.File != `share\album\song.mp3` {
		t.Fatalf("failed import lost fallback or recovery state: %+v, cleanup=%d", track, m.cleanups)
	}
}

func TestFailedTransferRetriesAnotherSource(t *testing.T) {
	track := &models.Track{ID: "id", CleanTitle: "song"}
	m := &testMonitor{statuses: []string{"Errored", "Succeeded"}}
	if err := (&DownloadClient{}).MonitorDownloads([]*models.Track{track}, m); err != nil {
		t.Fatal(err)
	}
	if !track.Present || m.retries != 1 || m.cleanups != 2 {
		t.Fatalf("retry failed: %+v, %+v", track, m)
	}
}

func TestMissingTransferRetriesAnotherSource(t *testing.T) {
	track := &models.Track{ID: "id", CleanTitle: "song"}
	m := &testMonitor{statuses: []string{"missing", "missing", "Succeeded"}}
	if err := (&DownloadClient{}).MonitorDownloads([]*models.Track{track}, m); err != nil {
		t.Fatal(err)
	}
	if !track.Present || m.retries != 1 {
		t.Fatalf("missing queue retry failed: %+v", m)
	}
}

func TestExhaustedSourcesRemainEligibleForFallback(t *testing.T) {
	track := &models.Track{ID: "id", CleanTitle: "song"}
	m := &testMonitor{statuses: []string{"Errored"}}
	if err := (&DownloadClient{}).MonitorDownloads([]*models.Track{track}, m); err != nil {
		t.Fatal(err)
	}
	if track.Present || m.retries != 2 {
		t.Fatalf("exhausted retry failed: %+v", m)
	}
}

func TestMaximumDurationDoesNotRestartOnRetry(t *testing.T) {
	track := &models.Track{ID: "id", CleanTitle: "song"}
	m := &testMonitor{statuses: []string{"Errored"}, max: time.Nanosecond}
	if err := (&DownloadClient{}).MonitorDownloads([]*models.Track{track}, m); err != nil {
		t.Fatal(err)
	}
	if track.Present || m.retries != 1 {
		t.Fatalf("total deadline was not enforced: %+v", m)
	}
}

func TestSlskdRetriesDistinctPeersAndHandlesEmptyQueue(t *testing.T) {
	var peers []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			peers = append(peers, r.URL.Path)
			w.Write([]byte(`[]`))
			return
		}
		if r.Method == "GET" {
			w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewSlskd(config.Slskd{URL: srv.URL, Timeout: 5, DownloadAttempts: 3, Filters: config.Filters{Extensions: []string{"mp3"}}}, t.TempDir())
	files, err := c.filterFiles([]File{
		{Username: "first", Name: "one.mp3", Extension: "mp3"},
		{Username: "first", Name: "copy.mp3", Extension: "mp3"},
		{Username: "second", Name: "two.mp3", Extension: "mp3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	track := &models.Track{ID: "search", CleanTitle: "song"}
	c.candidates[track] = files
	for i := 0; i < 2; i++ {
		if ok, err := c.RetryDownload(track); err != nil || !ok {
			t.Fatalf("retry %d: %v %v", i, ok, err)
		}
	}
	if ok, err := c.RetryDownload(track); err != nil || ok {
		t.Fatalf("exhaustion: %v %v", ok, err)
	}
	if !reflect.DeepEqual(peers, []string{"/api/v0/transfers/downloads/first", "/api/v0/transfers/downloads/second"}) {
		t.Fatalf("peers: %v", peers)
	}
	if statuses, err := c.GetDownloadStatus([]*models.Track{track}); err != nil || len(statuses) != 0 {
		t.Fatalf("empty queue: %v %v", statuses, err)
	}
}

func TestStalledTransferRetriesAnotherSource(t *testing.T) {
	track := &models.Track{ID: "id", CleanTitle: "song"}
	m := &testMonitor{statuses: []string{"Queued", "Queued", "Succeeded"}, stall: time.Nanosecond}
	if err := (&DownloadClient{}).MonitorDownloads([]*models.Track{track}, m); err != nil {
		t.Fatal(err)
	}
	if !track.Present || m.retries != 1 {
		t.Fatalf("stall did not retry: %+v", m)
	}
}

func TestMaximumDurationStopsEvenWhileBytesIncrease(t *testing.T) {
	track := &models.Track{ID: "id", CleanTitle: "song"}
	m := &testMonitor{statuses: []string{"Downloading"}, max: time.Nanosecond, bytesPerPoll: 1}
	if err := (&DownloadClient{}).MonitorDownloads([]*models.Track{track}, m); err != nil {
		t.Fatal(err)
	}
	if track.Present || m.retries != 0 || m.polls != 2 {
		t.Fatalf("progress bypassed deadline: %+v", m)
	}
}
