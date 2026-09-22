package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"

	"explo/src/config"
	"explo/src/models"
	"explo/src/util"
)

type playlistServer struct {
	mu                                sync.Mutex
	playlists                         []Playlist
	songs                             []string
	saves, creates, metadata, deletes int
	failLookup, failSave              bool
}

func (s *playlistServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload := map[string]any{"status": "ok"}
	q := r.URL.Query()
	switch r.URL.Path {
	case "/rest/getPlaylists":
		if s.failLookup {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		payload["playlists"] = map[string]any{"playlist": s.playlists}
	case "/rest/createPlaylist":
		s.saves++
		if s.failSave {
			payload["status"] = "failed"
			payload["error"] = map[string]any{"message": "write failed"}
			break
		}
		id := q.Get("playlistId")
		if id == "" {
			s.creates++
			id = "persistent-id"
			s.playlists = append(s.playlists, Playlist{ID: id, Name: q.Get("name"), Owner: "jeff"})
		}
		s.songs = q["songId"]
		payload["playlist"] = map[string]any{"id": id}
	case "/rest/updatePlaylist":
		s.metadata++
	case "/rest/deletePlaylist":
		s.deletes++
	default:
		http.NotFound(w, r)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"subsonic-response": payload})
}

func testSubsonic(t *testing.T, state *playlistServer) *Subsonic {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(state.handle))
	t.Cleanup(server.Close)
	return NewSubsonic(config.ClientConfig{URL: server.URL, PlaylistName: "Weekly-Exploration", Creds: config.Credentials{User: "jeff"}}, util.NewHttp(util.HttpClientConfig{Timeout: 5}))
}
func song(id string) *models.Track { return &models.Track{ID: id, Present: true} }

func TestSubsonicKeepsPlaylistIDAndReplacesSongs(t *testing.T) {
	state := &playlistServer{playlists: []Playlist{{ID: "original", Name: "Weekly-Exploration", Owner: "jeff", Public: true, Comment: "my description"}}, songs: []string{"old"}}
	c := testSubsonic(t, state)
	for _, tracks := range [][]*models.Track{{song("first"), song("second")}, {song("second"), song("third")}} {
		if err := c.SavePlaylist(tracks); err != nil {
			t.Fatal(err)
		}
		if c.Cfg.PlaylistID != "original" {
			t.Fatalf("changed playlist ID: %s", c.Cfg.PlaylistID)
		}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.creates != 0 || state.deletes != 0 || state.metadata != 0 || state.saves != 2 {
		t.Fatalf("unexpected playlist mutation: %+v", state)
	}
	if !reflect.DeepEqual(state.songs, []string{"second", "third"}) {
		t.Fatalf("appended instead of replaced: %v", state.songs)
	}
}

func TestSubsonicCreatesAbsentPlaylistOnlyOnce(t *testing.T) {
	state := &playlistServer{}
	c := testSubsonic(t, state)
	for i := 0; i < 2; i++ {
		if err := c.SavePlaylist([]*models.Track{song("song")}); err != nil {
			t.Fatal(err)
		}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.creates != 1 || state.metadata != 1 || state.deletes != 0 {
		t.Fatalf("expected one creation and metadata initialization: %+v", state)
	}
}

func TestSubsonicLookupFailureNeverCreatesPlaylist(t *testing.T) {
	state := &playlistServer{failLookup: true}
	c := testSubsonic(t, state)
	if err := c.SavePlaylist([]*models.Track{song("song")}); err == nil {
		t.Fatal("expected lookup error")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.saves != 0 || state.deletes != 0 {
		t.Fatalf("mutated after lookup failure: %+v", state)
	}
}

func TestSubsonicRejectsAmbiguousAndReadOnlyPlaylists(t *testing.T) {
	for _, playlists := range [][]Playlist{
		{{ID: "one", Name: "Weekly-Exploration", Owner: "jeff"}, {ID: "two", Name: "Weekly-Exploration", Owner: "jeff"}},
		{{ID: "one", Name: "Weekly-Exploration", Owner: "jeff", ReadOnly: true}},
	} {
		state := &playlistServer{playlists: playlists}
		c := testSubsonic(t, state)
		if err := c.SavePlaylist([]*models.Track{song("song")}); err == nil {
			t.Fatal("expected ambiguous/read-only error")
		}
		state.mu.Lock()
		saves := state.saves
		state.mu.Unlock()
		if saves != 0 {
			t.Fatal("mutated ambiguous/read-only playlist")
		}
	}
}

func TestSubsonicDoesNotOverwriteAnotherOwnersPlaylist(t *testing.T) {
	state := &playlistServer{playlists: []Playlist{{ID: "someone-else", Name: "Weekly-Exploration", Owner: "other"}}}
	c := testSubsonic(t, state)
	if err := c.SavePlaylist([]*models.Track{song("song")}); err != nil {
		t.Fatal(err)
	}
	if c.Cfg.PlaylistID == "someone-else" {
		t.Fatal("overwrote another owner's playlist")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.creates != 1 {
		t.Fatal("did not create owned playlist")
	}
}

func TestSubsonicSaveFailureDoesNotDeleteOrRecreate(t *testing.T) {
	state := &playlistServer{playlists: []Playlist{{ID: "original", Name: "Weekly-Exploration", Owner: "jeff"}}, failSave: true, songs: []string{"old"}}
	c := testSubsonic(t, state)
	if err := c.SavePlaylist([]*models.Track{song("new")}); err == nil {
		t.Fatal("expected save error")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.creates != 0 || state.deletes != 0 || !reflect.DeepEqual(state.songs, []string{"old"}) {
		t.Fatalf("lost original after save failure: %+v", state)
	}
}

func TestSubsonicEmptyOrUnresolvedTracksLeavePlaylistUntouched(t *testing.T) {
	state := &playlistServer{}
	c := testSubsonic(t, state)
	for _, tracks := range [][]*models.Track{nil, {{ID: "source-id", Present: false}}, {{Present: true}}} {
		if err := c.SavePlaylist(tracks); err == nil {
			t.Fatal("expected empty track error")
		}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.saves != 0 {
		t.Fatal("saved empty playlist")
	}
}

// Lifecycle fake ensures the main client defers mutations until the final save.
type persistentPlaylistAPI struct {
	calls     []string
	searchErr error
	resolve   bool
	saved     []*models.Track
}

func (a *persistentPlaylistAPI) GetLibrary() error { return nil }
func (a *persistentPlaylistAPI) GetAuth() error    { return nil }
func (a *persistentPlaylistAPI) AddHeader() error  { return nil }
func (a *persistentPlaylistAPI) AddLibrary() error { return nil }
func (a *persistentPlaylistAPI) SearchSongs(tracks []*models.Track) error {
	a.calls = append(a.calls, "search")
	if a.searchErr != nil {
		return a.searchErr
	}
	if a.resolve {
		tracks[0].Present = true
		tracks[0].ID = "library-id"
	}
	return nil
}
func (a *persistentPlaylistAPI) RefreshLibrary() error   { a.calls = append(a.calls, "scan"); return nil }
func (a *persistentPlaylistAPI) CheckRefreshState() bool { return true }
func (a *persistentPlaylistAPI) CreatePlaylist([]*models.Track) error {
	a.calls = append(a.calls, "create")
	return nil
}
func (a *persistentPlaylistAPI) SearchPlaylist() error {
	a.calls = append(a.calls, "lookup")
	return nil
}
func (a *persistentPlaylistAPI) UpdatePlaylist() error {
	a.calls = append(a.calls, "metadata")
	return nil
}
func (a *persistentPlaylistAPI) DeletePlaylist() error {
	a.calls = append(a.calls, "delete")
	return nil
}
func (a *persistentPlaylistAPI) SavePlaylist(tracks []*models.Track) error {
	a.calls = append(a.calls, "save")
	a.saved = tracks
	return nil
}

func TestPersistentPlaylistLifecycleDefersSaveAndFiltersUnresolvedTracks(t *testing.T) {
	api := &persistentPlaylistAPI{resolve: true}
	c := &Client{System: "subsonic", Cfg: &config.ClientConfig{}, API: api}
	if err := c.PreparePlaylist(true); err != nil {
		t.Fatal(err)
	}
	if len(api.calls) != 0 {
		t.Fatal("mutated playlist before acquisition")
	}
	if err := c.CreatePlaylist([]*models.Track{song("source-id"), song("stale-id")}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(api.calls, []string{"scan", "search", "save"}) {
		t.Fatalf("wrong lifecycle: %v", api.calls)
	}
	if len(api.saved) != 1 || api.saved[0].ID != "library-id" {
		t.Fatal("saved unresolved source IDs")
	}
}

func TestPersistentPlaylistLifecyclePreservesPlaylistOnResolutionFailure(t *testing.T) {
	for _, searchErr := range []error{nil, errors.New("library unavailable")} {
		api := &persistentPlaylistAPI{searchErr: searchErr}
		c := &Client{System: "subsonic", Cfg: &config.ClientConfig{}, API: api}
		if err := c.CreatePlaylist([]*models.Track{song("stale-id")}); err == nil {
			t.Fatal("expected unresolved track failure")
		}
		if !reflect.DeepEqual(api.calls, []string{"scan", "search"}) {
			t.Fatalf("mutated after failed resolution: %v", api.calls)
		}
	}
}

func TestSubsonicEncodesPlaylistAndSongIDs(t *testing.T) {
	id := "playlist&one"
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/getPlaylists" {
			json.NewEncoder(w).Encode(map[string]any{"subsonic-response": map[string]any{"status": "ok", "playlists": map[string]any{"playlist": []Playlist{{ID: id, Name: "Weekly-Exploration", Owner: "jeff"}}}}})
			return
		}
		got = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]any{"subsonic-response": map[string]any{"status": "ok", "playlist": map[string]string{"id": id}}})
	}))
	defer server.Close()
	c := NewSubsonic(config.ClientConfig{URL: server.URL, PlaylistName: "Weekly-Exploration", Creds: config.Credentials{User: "jeff"}}, util.NewHttp(util.HttpClientConfig{Timeout: 5}))
	if err := c.SavePlaylist([]*models.Track{song("track&one")}); err != nil {
		t.Fatal(err)
	}
	if got.Get("playlistId") != id || got.Get("songId") != "track&one" || got.Get("name") != "" {
		t.Fatalf("incorrect encoded update: %v", got)
	}
}
