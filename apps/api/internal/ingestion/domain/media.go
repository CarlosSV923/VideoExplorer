package domain

import "time"

// MediaInfo is what the technical reading of a video says about it (RF-24, RF-44):
// its container, duration and size, its main video track, and its audio and internal
// subtitle tracks. A value that the file does not report stays at its zero value
// (an empty language or title, a zero duration).
type MediaInfo struct {
	Container string
	Duration  time.Duration
	Size      int64
	Video     VideoTrack
	Audio     []AudioTrack
	Subtitles []SubtitleTrack
}

// VideoTrack is the main video track of a file: the first one that is not cover art.
type VideoTrack struct {
	Codec         string
	Width, Height int
	// HDR is true when the stream signals a high dynamic range transfer (PQ or HLG)
	// or carries a Dolby Vision configuration.
	HDR bool
}

// AudioTrack is one audio track, in the order the file holds them.
type AudioTrack struct {
	Codec    string
	Channels int
	Language string
	Title    string
	Default  bool
}

// SubtitleTrack is one subtitle track stored inside the file (RF-23).
type SubtitleTrack struct {
	Codec    string
	Language string
	Title    string
	Forced   bool
	// SDH is the track for the hard of hearing (hearing impaired flag).
	SDH     bool
	Default bool
}

// FileIdentity tells one file's content apart on its disk. Device and Inode name the
// file and survive a rename inside one file system (RNF-04); Size and ModTime change
// when its content does.
type FileIdentity struct {
	Device, Inode uint64
	Size          int64
	ModTime       time.Time
}

// Matches reports whether a probe taken from id still describes the file that is now
// current: the same device and inode, and the same size and modification time. A change
// to the size or the modification time makes the probe stale (RF-24 cache).
func (id FileIdentity) Matches(current FileIdentity) bool {
	return id.Device == current.Device && id.Inode == current.Inode &&
		id.Size == current.Size && id.ModTime.Equal(current.ModTime)
}
