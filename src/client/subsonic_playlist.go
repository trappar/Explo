package client

import (
	"fmt"
	"net/url"

	"explo/src/models"
	"explo/src/util"
)

// SavePlaylist replaces the song list using createPlaylist's playlistId form.
// Despite the endpoint name, this preserves the existing playlist's identity.
// See https://opensubsonic.netlify.app/docs/endpoints/createplaylist/ .
func (c *Subsonic) SavePlaylist(tracks []*models.Track) error {
	values := url.Values{"f": {"json"}}
	for _, track := range tracks {
		if track.Present && track.ID != "" {
			values.Add("songId", track.ID)
		}
	}
	if len(values["songId"]) == 0 {
		return fmt.Errorf("no library tracks available; leaving playlist unchanged")
	}

	// Always look up the latest ID, and never interpret a failed lookup as absence.
	c.Cfg.PlaylistID = ""
	body, err := c.subsonicRequest("getPlaylists?f=json")
	if err != nil {
		return err
	}
	var existing SubResponse
	if err := util.ParseResp(body, &existing); err != nil {
		return err
	}
	for _, playlist := range existing.SubsonicResponse.Playlists.Playlist {
		if playlist.Name != c.Cfg.PlaylistName {
			continue
		}
		// Public playlists from other owners must not be overwritten.
		if playlist.Owner != "" && playlist.Owner != c.Cfg.Creds.User {
			continue
		}
		if playlist.ReadOnly {
			return fmt.Errorf("playlist %q is read-only", playlist.Name)
		}
		if playlist.ID == "" {
			return fmt.Errorf("playlist %q has no ID", playlist.Name)
		}
		if c.Cfg.PlaylistID != "" {
			return fmt.Errorf("multiple owned playlists named %q; cannot choose one safely", playlist.Name)
		}
		c.Cfg.PlaylistID = playlist.ID
	}

	previousID := c.Cfg.PlaylistID
	if previousID != "" {
		values.Set("playlistId", previousID)
	} else {
		values.Set("name", c.Cfg.PlaylistName)
	}
	body, err = c.subsonicRequest("createPlaylist?" + values.Encode())
	if err != nil {
		return err
	}
	var saved SubResponse
	if err := util.ParseResp(body, &saved); err != nil {
		return err
	}
	if saved.SubsonicResponse.Playlist.ID == "" {
		return fmt.Errorf("playlist save returned no ID")
	}
	if previousID != "" && saved.SubsonicResponse.Playlist.ID != previousID {
		return fmt.Errorf("playlist save unexpectedly changed ID from %q to %q", previousID, saved.SubsonicResponse.Playlist.ID)
	}
	c.Cfg.PlaylistID = saved.SubsonicResponse.Playlist.ID
	if previousID == "" {
		// Set defaults on creation; leave user-edited metadata alone on later runs.
		return c.UpdatePlaylist()
	}
	return nil
}
